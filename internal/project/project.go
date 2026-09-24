package project

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Project é um Manifest resolvido contra o disco.
type Project struct {
	Manifest
	ID          string `json:"id"`          // NormalizeName(Name)
	Root        string `json:"root"`        // absoluto, limpo
	HasManifest bool   `json:"hasManifest"` // hyphp.yaml existia
	HasHtaccess bool   `json:"hasHtaccess"` // .htaccess em Root ou em DocrootAbs
	HasIndex    bool   `json:"hasIndex"`    // index.php/html/htm em DocrootAbs
	DocrootAbs  string `json:"docrootAbs"`  // Root ou Root/Docroot
	// PHPEffective é a série que o projeto usa de fato, já resolvida pela
	// precedência manifesto → DefaultPHP → maior instalada. Vazia até o Stack
	// publicar os projetos. A UI lê daqui em vez de repetir a regra.
	PHPEffective string `json:"phpEffective"`
}

// diretórios que nunca são projetos, mesmo com index.php dentro.
var skipDirs = map[string]bool{"node_modules": true, "vendor": true}

// Load lê root/hyphp.yaml (se existir), aplica defaults e valida. A ausência
// do yaml nunca é erro; yaml malformado ou inválido é.
func Load(root string) (Project, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Project{}, fmt.Errorf("project: resolver %q: %w", root, err)
	}
	abs = filepath.Clean(abs)

	p := Project{Root: abs}
	raw, err := os.ReadFile(filepath.Join(abs, ManifestFile))
	switch {
	case err == nil:
		p.HasManifest = true
		if err := yaml.Unmarshal(raw, &p.Manifest); err != nil {
			return Project{}, fmt.Errorf("project: %s inválido em %s: %w", ManifestFile, abs, err)
		}
	case errors.Is(err, os.ErrNotExist):
		// sem manifesto: só defaults
	default:
		return Project{}, fmt.Errorf("project: ler %s em %s: %w", ManifestFile, abs, err)
	}

	ApplyDefaults(&p.Manifest, abs)
	if err := Validate(p.Manifest); err != nil {
		return Project{}, fmt.Errorf("project: %s: %w", abs, err)
	}

	p.ID = NormalizeName(p.Name)
	p.DocrootAbs = abs
	if p.Docroot != "" {
		p.DocrootAbs = filepath.Join(abs, filepath.FromSlash(p.Docroot))
	}
	p.HasHtaccess = fileExists(filepath.Join(abs, ".htaccess")) || fileExists(filepath.Join(p.DocrootAbs, ".htaccess"))
	// Sem índice no docroot, o domínio abre um 404 mesmo com tudo certo: o
	// vhost casa, o servidor entra no diretório e não acha o que servir. Aqui é
	// onde a informação existe barato — o stat já está sendo feito.
	for _, nome := range []string{"index.php", "index.html", "index.htm"} {
		if fileExists(filepath.Join(p.DocrootAbs, nome)) {
			p.HasIndex = true
			break
		}
	}
	return p, nil
}

// Discover varre os filhos diretos de cada root. Um diretório é projeto se tiver
// hyphp.yaml, public/index.php, index.php ou composer.json. Ignora nomes que
// começam com "." e node_modules/vendor. Roots inexistentes são ignorados.
// IDs duplicados: o primeiro (na ordem de roots) vence. Resultado ordenado por ID.
func Discover(roots []string) ([]Project, error) {
	seen := map[string]bool{}
	var out []Project
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("project: listar %s: %w", root, err)
		}
		for _, e := range entries {
			if !e.IsDir() || e.Name()[0] == '.' || skipDirs[e.Name()] {
				continue
			}
			dir := filepath.Join(root, e.Name())
			if !isProjectDir(dir) {
				continue
			}
			p, err := Load(dir)
			if err != nil {
				return nil, err
			}
			if seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func isProjectDir(dir string) bool {
	return fileExists(filepath.Join(dir, ManifestFile)) ||
		fileExists(filepath.Join(dir, "public", "index.php")) ||
		fileExists(filepath.Join(dir, "index.php")) ||
		fileExists(filepath.Join(dir, "composer.json"))
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// manifestHeader é curto de propósito: o arquivo vai para o repositório do
// usuário, e caminho de spec interna e lição sobre versionar não têm lugar lá.
const manifestHeader = "# Configuração do projeto no HyPHP.\n"

// Write grava Root/hyphp.yaml a partir de p.Manifest, com cabeçalho comentado e
// as chaves na ordem da struct (yaml.v3 preserva a ordem dos campos).
// Escrita atômica: tmp + rename.
func (p Project) Write() error {
	var buf bytes.Buffer
	buf.WriteString(manifestHeader)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(p.Manifest); err != nil {
		return fmt.Errorf("project: serializar manifesto de %s: %w", p.ID, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("project: fechar encoder: %w", err)
	}

	dst := filepath.Join(p.Root, ManifestFile)
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("project: gravar %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("project: renomear %s → %s: %w", tmp, dst, err)
	}
	return nil
}
