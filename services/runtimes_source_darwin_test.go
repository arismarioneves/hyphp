package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"hyphp/internal/brew"
	"hyphp/internal/i18n"
	"hyphp/internal/pkgmgr"
	"hyphp/internal/runtime"
)

// catalogoMac imita o catálogo embutido: builds Windows e o phpMyAdmin.
var catalogoMac = pkgmgr.Catalog{Packages: []pkgmgr.Package{
	{ID: "php-8.3.35-nts-vs16-x64", Kind: runtime.PHP, Version: "8.3.35"},
	{ID: "phpmyadmin-5.2.3", Kind: runtime.PhpMyAdmin, Version: "5.2.3"},
}}

func servicoComBrew(t *testing.T, locate func(context.Context) (brew.Brew, error)) *RuntimesService {
	t.Helper()
	r := NewRuntimesService(RuntimesDeps{BinDir: t.TempDir(), Catalog: catalogoMac})
	r.src.locate = locate
	return r
}

// Um php@8.3 já presente em opt/ (mesmo instalado pelo terminal) não pode
// ser oferecido: o `brew install` do tap falharia como "já instalado".
func TestAvailableEscondeFormulaComKegEmOpt(t *testing.T) {
	prefix := t.TempDir()
	if err := os.MkdirAll(filepath.Join(prefix, "opt", "php@8.3"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := servicoComBrew(t, func(context.Context) (brew.Brew, error) {
		return brew.Brew{Exe: filepath.Join(prefix, "bin", "brew"), Prefix: prefix}, nil
	})

	ids := map[string]bool{}
	for _, p := range r.Available() {
		ids[p.ID] = true
	}
	if ids["shivammathur/php/php@8.3"] {
		t.Error("php@8.3 oferecido apesar de opt/php@8.3 existir")
	}
	if !ids["shivammathur/php/php@8.4"] {
		t.Error("php@8.4 deveria ser oferecido")
	}
	if !ids["phpmyadmin-5.2.3"] {
		t.Error("phpMyAdmin do catálogo deveria continuar oferecido")
	}
	if ids["php-8.3.35-nts-vs16-x64"] {
		t.Error("build Windows do catálogo oferecida no Mac")
	}
}

// Sem Homebrew a instalação de fórmula falha na hora com a mensagem que
// aponta o banner, e Homebrew() entrega o comando para copiar.
func TestSemHomebrewInstallRecusaEStatusTrazComando(t *testing.T) {
	r := servicoComBrew(t, func(context.Context) (brew.Brew, error) {
		return brew.Brew{}, brew.ErrNotFound
	})

	err := r.Install("httpd")
	if err == nil || err.Error() != i18n.Errorf("err.brew.missing").Error() {
		t.Fatalf("Install(httpd) = %v; quer err.brew.missing", err)
	}
	want := BrewStatus{Supported: true, Found: false, InstallCommand: brew.InstallCommand}
	if got := r.Homebrew(); got != want {
		t.Fatalf("Homebrew() = %+v; quer %+v", got, want)
	}
}
