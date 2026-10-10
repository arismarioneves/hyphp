package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"hyphp/internal/brew"
	"hyphp/internal/i18n"
	"hyphp/internal/pkgmgr"
	"hyphp/internal/runtime"
)

// sourceState guarda o Homebrew achado na última varredura. b e found ficam
// sob r.mu; locate é fixo depois do construtor (os testes trocam por um
// Homebrew falso num prefixo temporário).
type sourceState struct {
	locate func(ctx context.Context) (brew.Brew, error)
	b      brew.Brew
	found  bool
}

func newSourceState() sourceState { return sourceState{locate: brew.Locate} }

// scan procura o Homebrew de novo a cada varredura (quem instala o Homebrew
// com o app aberto vê o resultado no "Verificar de novo"), varre os kegs de
// <prefix>/opt e, em bin/, o que vem do catálogo (só o phpMyAdmin no Mac).
func (r *RuntimesService) scan() ([]runtime.Installed, error) {
	b, err := r.src.locate(r.ctx)
	found := err == nil
	r.mu.Lock()
	changed := found != r.src.found || b.Prefix != r.src.b.Prefix
	r.src.b, r.src.found = b, found
	r.mu.Unlock()
	if changed {
		// O watcher precisa passar a observar o <prefix>/opt novo; vale
		// também para o Rescan da UI, que não passa pelo laço do watcher.
		select {
		case r.watchRefresh <- struct{}{}:
		default:
		}
	}

	var list []runtime.Installed
	var errs []error
	switch {
	case err == nil:
		kegs, err := b.Scan(r.ctx)
		list = append(list, kegs...)
		if err != nil {
			errs = append(errs, err)
		}
	case !errors.Is(err, brew.ErrNotFound):
		// Homebrew ausente é estado normal (a UI mostra o banner); outra
		// falha do Locate merece ir para o log.
		errs = append(errs, err)
	}
	inBin, err := runtime.Scan(r.d.BinDir)
	list = append(list, inBin...)
	if err != nil {
		errs = append(errs, err)
	}
	return list, errors.Join(errs...)
}

// brewNow devolve o Homebrew da última varredura, varrendo antes se ainda
// não houve nenhuma (Install/Homebrew podem chegar antes de Installed).
func (r *RuntimesService) brewNow() (brew.Brew, bool) {
	r.Installed()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.src.b, r.src.found
}

// formulaByName acha a fórmula da tabela pelo nome de instalação, que é
// também o ID do pacote oferecido à UI.
func formulaByName(name string) (brew.Formula, bool) {
	for _, f := range brew.Formulas {
		if f.Name == name {
			return f, true
		}
	}
	return brew.Formula{}, false
}

// available oferece as fórmulas da tabela ainda ausentes e o phpMyAdmin do
// catálogo; o resto do catálogo embutido é de builds Windows. A fórmula some
// pelo keg em <prefix>/opt, não por Kind@Version: um php@8.3 instalado por
// fora do HyPHP (até do homebrew-core) faria o `brew install` do tap falhar
// como "já instalado".
func (r *RuntimesService) available(installed []runtime.Installed) []pkgmgr.Package {
	r.mu.Lock()
	b, found := r.src.b, r.src.found
	r.mu.Unlock()

	formulas := map[string]bool{}
	have := map[string]bool{}
	for _, i := range installed {
		if i.Formula != "" {
			formulas[i.Formula] = true
		}
		have[string(i.Kind)+"@"+i.Version] = true
	}
	var out []pkgmgr.Package
	for _, f := range brew.Formulas {
		if formulas[f.Name] {
			continue
		}
		// Só o nome curto: opt/php pode ser outra série (alias do core) e
		// não deve esconder o php@8.5.
		if found {
			if _, err := os.Lstat(filepath.Join(b.Prefix, "opt", f.Short())); err == nil {
				continue
			}
		}
		out = append(out, pkgmgr.Package{ID: f.Name, Kind: f.Kind, Version: f.Series, Formula: f.Name})
	}
	for _, p := range r.d.Catalog().ByKind(runtime.PhpMyAdmin) {
		if !have[string(p.Kind)+"@"+p.Version] {
			out = append(out, p)
		}
	}
	return out
}

// installer resolve primeiro na tabela do Homebrew, depois o phpMyAdmin do
// catálogo. brew.Install não emite a fase final; ela sai daqui, com o
// PackageID, como o Manager faz no download do catálogo.
func (r *RuntimesService) installer(packageID string) (installFunc, error) {
	if f, ok := formulaByName(packageID); ok {
		b, found := r.brewNow()
		if !found {
			return nil, i18n.Errorf("err.brew.missing")
		}
		return func(ctx context.Context, on func(pkgmgr.Progress)) error {
			err := b.Install(ctx, f, func(p pkgmgr.Progress) {
				p.PackageID = packageID
				on(p)
			})
			final := pkgmgr.Progress{PackageID: packageID, Phase: pkgmgr.PhaseDone}
			switch {
			case errors.Is(err, context.Canceled):
				final.Phase = pkgmgr.PhaseCanceled
			case errors.Is(err, brew.ErrBusy):
				err = i18n.Errorf("err.brew.busy")
				final.Phase, final.Error = pkgmgr.PhaseError, err.Error()
			case err != nil:
				final.Phase, final.Error = pkgmgr.PhaseError, err.Error()
			}
			on(final)
			return err
		}, nil
	}
	pkg, ok := r.d.Catalog().ByID(packageID)
	if !ok || pkg.Kind != runtime.PhpMyAdmin {
		return nil, i18n.Errorf("err.runtimes.unknownPackage", packageID)
	}
	return func(ctx context.Context, on func(pkgmgr.Progress)) error {
		_, err := r.d.Manager.Install(ctx, pkg, on)
		return err
	}, nil
}

// remove: keg do Homebrew sai por `brew uninstall`; o resto (phpMyAdmin)
// é pasta em bin/.
func (r *RuntimesService) remove(inst runtime.Installed) error {
	if inst.Formula == "" {
		return r.d.Manager.Remove(inst)
	}
	f, ok := formulaByName(inst.Formula)
	if !ok {
		f = brew.Formula{Name: inst.Formula, Kind: inst.Kind}
	}
	b, found := r.brewNow()
	if !found {
		return i18n.Errorf("err.brew.missing")
	}
	err := b.Uninstall(r.ctx, f)
	if errors.Is(err, brew.ErrBusy) {
		return i18n.Errorf("err.brew.busy")
	}
	return err
}

// Homebrew informa à UI se o Homebrew foi achado; sem ele, a tela mostra
// InstallCommand para copiar (o HyPHP não roda o instalador).
func (r *RuntimesService) Homebrew() BrewStatus {
	b, found := r.brewNow()
	return BrewStatus{Supported: true, Found: found, Prefix: b.Prefix, InstallCommand: brew.InstallCommand}
}

// extraWatchDirs: <prefix>/opt, onde os kegs aparecem e somem como symlinks
// (instalação pelo terminal, `brew upgrade` repontando).
func (r *RuntimesService) extraWatchDirs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.src.found {
		return nil
	}
	return []string{filepath.Join(r.src.b.Prefix, "opt")}
}

// PickImportDir: no Mac os runtimes vêm do Homebrew; a UI esconde o botão.
func (r *RuntimesService) PickImportDir() (string, error) {
	return "", i18n.Errorf("err.runtimes.importMac")
}

// ImportFrom: idem PickImportDir.
func (r *RuntimesService) ImportFrom(dir string) error {
	return i18n.Errorf("err.runtimes.importMac")
}
