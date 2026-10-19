//go:build darwin || linux

package services

import (
	"os"
	"path/filepath"
	"testing"

	"hyphp/internal/paths"
	"hyphp/internal/runtime"
)

func raizTemp(t *testing.T) {
	t.Helper()
	t.Setenv(paths.EnvRoot, t.TempDir())
}

// Ao abrir, os atalhos acompanham o app e a série padrão: o hyphp aponta para
// o bundle atual e o php-bin para a pasta bin do PHP padrão.
func TestAtalhosSeguemOAppEOPHPPadrao(t *testing.T) {
	raizTemp(t)
	dir := paths.Cli()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/opt/homebrew/opt/php@8.2/bin", filepath.Join(dir, paths.PhpBin)); err != nil {
		t.Fatal(err)
	}
	php := func() (runtime.Installed, bool) {
		return runtime.Installed{Kind: runtime.PHP, Dir: "/opt/homebrew/opt/php@8.3"}, true
	}
	if err := RefreshPathLinks("/Applications/HyPHP.app/Contents/Helpers/hyphp", php); err != nil {
		t.Fatal(err)
	}
	for link, want := range map[string]string{
		"hyphp":      "/Applications/HyPHP.app/Contents/Helpers/hyphp",
		paths.PhpBin: "/opt/homebrew/opt/php@8.3/bin",
	} {
		if got, err := os.Readlink(filepath.Join(dir, link)); err != nil || got != want {
			t.Errorf("%s -> (%q, %v), quer %q", link, got, err, want)
		}
	}
}

// Quem nunca pediu PATH não ganha pasta, e quem pediu só a CLI não ganha php.
func TestAtalhosRespeitamQuemNaoPediu(t *testing.T) {
	raizTemp(t)
	php := func() (runtime.Installed, bool) { return runtime.Installed{Dir: "/opt/homebrew/opt/php@8.3"}, true }
	if err := RefreshPathLinks("/x/hyphp", php); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.Cli()); !os.IsNotExist(err) {
		t.Fatalf("pasta cli criada sem pedido: %v", err)
	}
	if err := os.MkdirAll(paths.Cli(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RefreshPathLinks("/x/hyphp", php); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(paths.Cli(), paths.PhpBin)); !os.IsNotExist(err) {
		t.Fatalf("php-bin criado sem o botão PHP no PATH: %v", err)
	}
}

// O php que o Terminal acha só é aviso quando não é o do HyPHP.
func TestPHPNaFrente(t *testing.T) {
	cli := "/Users/x/Library/Application Support/HyPHP/cli"
	for found, want := range map[string]string{
		"":                         "",
		cli + "/php-bin/php":       "",
		"/opt/homebrew/bin/php":    "/opt/homebrew/bin/php",
		cli + "-velho/php-bin/php": cli + "-velho/php-bin/php",
	} {
		if got := phpShadow(found, cli); got != want {
			t.Errorf("phpShadow(%q) = %q, quer %q", found, got, want)
		}
	}
}
