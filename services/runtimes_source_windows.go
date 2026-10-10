package services

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"hyphp/internal/i18n"
	"hyphp/internal/pkgmgr"
	"hyphp/internal/runtime"
)

// sourceState: no Windows tudo vem de bin/ e do catálogo; não há estado de
// fonte além de RuntimesDeps.
type sourceState struct{}

func newSourceState() sourceState { return sourceState{} }

// scan varre bin/.
func (r *RuntimesService) scan() ([]runtime.Installed, error) {
	return runtime.Scan(r.d.BinDir)
}

// available devolve os pacotes do catálogo cuja (Kind, Version) não está instalada.
func (r *RuntimesService) available(installed []runtime.Installed) []pkgmgr.Package {
	have := make(map[string]bool, len(installed))
	for _, i := range installed {
		have[string(i.Kind)+"@"+i.Version] = true
	}
	out := make([]pkgmgr.Package, 0, len(r.d.Catalog.Packages))
	for _, p := range r.d.Catalog.Packages {
		// O pacote de CAs não é runtime: o stack o baixa sozinho para o PHP.
		if p.Kind == pkgmgr.KindCACert {
			continue
		}
		if !have[string(p.Kind)+"@"+p.Version] {
			out = append(out, p)
		}
	}
	return out
}

// installer resolve o pacote do catálogo. O Manager já emite a fase final
// (done/error/canceled) com PackageID.
func (r *RuntimesService) installer(packageID string) (installFunc, error) {
	pkg, ok := r.d.Catalog.ByID(packageID)
	if !ok || pkg.Kind == pkgmgr.KindCACert {
		return nil, i18n.Errorf("err.runtimes.unknownPackage", packageID)
	}
	return func(ctx context.Context, on func(pkgmgr.Progress)) error {
		_, err := r.d.Manager.Install(ctx, pkg, on)
		return err
	}, nil
}

// remove apaga a pasta do runtime em bin/.
func (r *RuntimesService) remove(inst runtime.Installed) error {
	return r.d.Manager.Remove(inst)
}

// Homebrew: no Windows não há Homebrew; Supported=false também serve de
// sinal de plataforma para a UI.
func (r *RuntimesService) Homebrew() BrewStatus {
	return BrewStatus{}
}

// extraWatchDirs: no Windows bin/ basta.
func (r *RuntimesService) extraWatchDirs() []string { return nil }

// kindsImportaveis são os tipos que a importação aceita. Mailpit e mkcert ficam
// direto em bin/<kind>/, sem pasta por versão, e pesam poucos megabytes no
// catálogo: importá-los exigiria tratar um segundo layout de destino sem poupar
// download nenhum.
var kindsImportaveis = []runtime.Kind{runtime.PHP, runtime.Apache, runtime.Nginx, runtime.MySQL, runtime.MariaDB}

// copyRuntimeTree copia recursivamente src para dst. Copia, não move: a pasta
// de origem costuma ser de outra ferramenta que o usuário ainda usa, e o HyPHP
// não pode depender de uma instalação que não controla.
func copyRuntimeTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		alvo := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(alvo, 0o755)
		}
		// Links e dispositivos não interessam num runtime e só trariam
		// surpresa (um symlink para fora da árvore, por exemplo).
		if !d.Type().IsRegular() {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(alvo), 0o755); err != nil {
			return err
		}
		out, err := os.Create(alvo)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return out.Close()
	})
}

// destinoImport devolve a pasta de bin/ que vai receber inst.
func destinoImport(binDir string, inst runtime.Installed) string {
	nome := filepath.Base(inst.Dir)
	if strings.EqualFold(nome, string(inst.Kind)) {
		// O XAMPP guarda a build em <raiz>\php; copiada com o nome de origem
		// daria bin/php/php, que esconde a versão na lista e colidiria com a
		// próxima importação do mesmo tipo.
		nome = string(inst.Kind) + "-" + inst.Version
	}
	return filepath.Join(binDir, string(inst.Kind), nome)
}

// runtimesEm reconhece as builds sob dir. Cobre os três jeitos de o usuário
// apontar a pasta: uma raiz no layout do HyPHP/Laragon (<dir>/php/<build>), a
// pasta que contém as builds direto (<dir>/<build>, o bin\php do Laragon) e a
// própria build (ou a raiz do XAMPP, onde <dir>/php já é a build).
func runtimesEm(dir string) []runtime.Installed {
	var achadas []runtime.Installed
	vistas := map[string]bool{}
	add := func(inst runtime.Installed) {
		// Um bin/ no layout do HyPHP também traz mailpit e mkcert, que moram
		// direto em bin/<kind>/: copiados para bin/<kind>/<pasta> virariam
		// bytes que nenhuma varredura encontra.
		if !slices.Contains(kindsImportaveis, inst.Kind) || vistas[strings.ToLower(inst.Dir)] {
			return
		}
		vistas[strings.ToLower(inst.Dir)] = true
		achadas = append(achadas, inst)
	}

	// O erro do Scan só descreve builds que não rodam; elas simplesmente não
	// são importáveis, e falhar aqui esconderia as que deram certo.
	list, _ := runtime.Scan(dir)
	for _, inst := range list {
		add(inst)
	}

	candidatos := []string{dir}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				candidatos = append(candidatos, filepath.Join(dir, e.Name()))
			}
		}
	}
	for _, c := range candidatos {
		for _, kind := range kindsImportaveis {
			// Detect falha barato (os.Stat do executável) quando o candidato
			// não é daquele tipo.
			if inst, err := runtime.Detect(kind, c); err == nil {
				add(inst)
			}
		}
	}
	return achadas
}

// PickImportDir abre o diálogo nativo de pasta. Cancelar devolve ("", nil) —
// mesma comparação de mensagem de ProjectsService.PickRoot, pelo mesmo motivo
// (cfd é interno ao Wails, não há sentinela importável).
func (r *RuntimesService) PickImportDir() (string, error) {
	dir, err := r.d.App.Dialog.OpenFile().
		SetTitle(i18n.T("dialog.pickImportDir")).
		CanChooseDirectories(true).
		CanChooseFiles(false).
		PromptForSingleSelection()
	if err != nil {
		if strings.Contains(err.Error(), dialogCancelledMsg) {
			return "", nil
		}
		return "", i18n.Errorf("err.dialog", err)
	}
	return dir, nil
}

// ImportFrom copia para bin/ toda build reconhecível sob dir. Quem já tem PHP,
// Apache ou MySQL na máquina não precisa rebaixar centenas de megabytes.
func (r *RuntimesService) ImportFrom(dir string) error {
	achadas := runtimesEm(dir)
	if len(achadas) == 0 {
		return i18n.Errorf("err.runtimes.noneFound", dir)
	}
	for _, inst := range achadas {
		destino := destinoImport(r.d.BinDir, inst)
		if _, err := os.Stat(destino); err == nil {
			continue // já instalado: importar de novo só duplicaria bytes
		}
		if err := copyRuntimeTree(inst.Dir, destino); err != nil {
			// Uma árvore pela metade seria detectada como runtime quebrado na
			// próxima varredura; melhor não deixar rastro da cópia falha.
			os.RemoveAll(destino)
			return i18n.Errorf("err.runtimes.copy", inst.Dir, err)
		}
	}
	// O watcher de bin/ publica a lista nova; forçar o rescan evita depender
	// do tempo de propagação do evento de arquivo.
	if err := r.Rescan(); err != nil {
		return i18n.Errorf("err.runtimes.rescan", err)
	}
	return nil
}
