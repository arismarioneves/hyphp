// Package render grava configurações geradas em disco e produz os arquivos de
// configuração que não pertencem a um web server específico (php.ini, my.ini).
package render

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// KeepFile marca um diretório que precisa existir mas cujo conteúdo NÃO
// pertence ao HyPHP. Ver WriteFiles.
const KeepFile = ".keep"

// tmpPrefix é o prefixo dos temporários de WriteFiles. Fica visível para o
// varredor de obsoletos: um temporário órfão de uma execução interrompida é
// lixo e deve sumir.
const tmpPrefix = ".hyphp-"

type entry struct {
	rel  string // relativo a dir, sempre com "/"
	data []byte
}

// WriteFiles materializa files (caminho relativo com "/" → conteúdo) dentro de
// dir e devolve changed=true se o conteúdo de dir mudou.
//
// Garantias:
//   - Cada arquivo é publicado por tmp+rename no MESMO diretório, então um
//     leitor nunca vê arquivo pela metade.
//   - Arquivo idêntico ao que já está em disco não é reescrito e não conta
//     como mudança. É isso que permite ao stack reiniciar o web server só
//     quando a config realmente mudou.
//   - Arquivos obsoletos (presentes em dir, ausentes de files) são apagados,
//     inclusive dentro de subdiretórios. Sem isso um vhost de projeto removido
//     continuaria sendo lido pelo `IncludeOptional vhosts/*.conf`.
//   - Diretório que ficou vazio POR CAUSA de uma remoção desta chamada é
//     removido. Diretório que já estava vazio é deixado em paz — senão
//     `logs/`/`temp/` criados por stack.ensureWebDirs fariam changed=true em
//     todo Reconcile e reiniciariam o web server à toa.
//   - Um diretório que declara ".keep" em files é criado e depois IGNORADO
//     pela varredura: ".keep" declara existência, não posse. É assim que
//     `logs/` e `temp/` do nginx sobrevivem com os arquivos que o próprio
//     nginx cria lá dentro (logs/error.log, temp/client_body_temp/...).
//   - Nenhuma escrita ou remoção sai de dir: chave absoluta, com volume ou com
//     ".." é recusada antes de qualquer I/O.
func WriteFiles(dir string, files map[string][]byte) (bool, error) {
	if strings.TrimSpace(dir) == "" {
		return false, errors.New("render: dir vazio")
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]entry, 0, len(names))
	want := make(map[string]bool, len(names))
	protected := make(map[string]bool)
	for _, name := range names {
		rel, err := cleanRel(dir, name)
		if err != nil {
			return false, err
		}
		if want[rel] {
			return false, fmt.Errorf("render: chave duplicada após normalizar: %q", rel)
		}
		want[rel] = true
		entries = append(entries, entry{rel: rel, data: files[name]})
		if path.Base(rel) == KeepFile {
			if d := path.Dir(rel); d != "." {
				protected[d] = true
			}
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("render: criar %s: %w", dir, err)
	}

	changed := false
	for _, e := range entries {
		abs := filepath.Join(dir, filepath.FromSlash(e.rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return changed, fmt.Errorf("render: criar %s: %w", filepath.Dir(abs), err)
		}
		same, err := sameContent(abs, e.data)
		if err != nil {
			return changed, err
		}
		if same {
			continue
		}
		if err := writeAtomic(abs, e.data); err != nil {
			return changed, err
		}
		changed = true
	}

	swept, err := sweep(dir, "", want, protected)
	if err != nil {
		return changed || swept, err
	}
	return changed || swept, nil
}

// cleanRel normaliza a chave e prova que ela aponta para dentro de dir.
func cleanRel(dir, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("render: nome de arquivo vazio em %s", dir)
	}
	if filepath.VolumeName(name) != "" || strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return "", fmt.Errorf("render: %q é caminho absoluto; só caminhos relativos a %s são aceitos", name, dir)
	}
	rel := path.Clean(filepath.ToSlash(name))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("render: %q escapa de %s", name, dir)
	}
	// Prova independente da normalização acima: o caminho resolvido tem de
	// continuar sendo um filho de dir.
	back, err := filepath.Rel(dir, filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("render: %q não é relativo a %s: %w", name, dir, err)
	}
	if back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("render: %q escapa de %s", name, dir)
	}
	return rel, nil
}

// sweep apaga, dentro de root/rel, tudo que não está em want. Diretórios com
// ".keep" declarado são pulados inteiros.
func sweep(root, rel string, want, protected map[string]bool) (bool, error) {
	abs := root
	if rel != "" {
		abs = filepath.Join(root, filepath.FromSlash(rel))
	}
	items, err := os.ReadDir(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("render: ler %s: %w", abs, err)
	}

	changed := false
	for _, item := range items {
		child := item.Name()
		if rel != "" {
			child = rel + "/" + item.Name()
		}
		childAbs := filepath.Join(root, filepath.FromSlash(child))

		if item.IsDir() {
			if protected[child] {
				continue
			}
			sub, err := sweep(root, child, want, protected)
			if err != nil {
				return changed || sub, err
			}
			if !sub {
				continue
			}
			changed = true
			rest, err := os.ReadDir(childAbs)
			if err != nil {
				return changed, fmt.Errorf("render: ler %s: %w", childAbs, err)
			}
			if len(rest) > 0 {
				continue
			}
			if err := os.Remove(childAbs); err != nil {
				return changed, fmt.Errorf("render: remover diretório vazio %s: %w", childAbs, err)
			}
			continue
		}

		if want[child] {
			continue
		}
		if err := os.Remove(childAbs); err != nil {
			return changed, fmt.Errorf("render: remover obsoleto %s: %w", childAbs, err)
		}
		changed = true
	}
	return changed, nil
}

func sameContent(abs string, want []byte) (bool, error) {
	got, err := os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("render: ler %s: %w", abs, err)
	}
	return bytes.Equal(got, want), nil
}

func writeAtomic(dest string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(dest), tmpPrefix+"*.tmp")
	if err != nil {
		return fmt.Errorf("render: criar temporário para %s: %w", dest, err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("render: gravar %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return fmt.Errorf("render: fechar %s: %w", name, err)
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return fmt.Errorf("render: permissões de %s: %w", name, err)
	}
	if err := os.Rename(name, dest); err != nil {
		os.Remove(name)
		return fmt.Errorf("render: publicar %s: %w", dest, err)
	}
	return nil
}
