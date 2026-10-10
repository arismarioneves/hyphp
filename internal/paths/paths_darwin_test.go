package paths

import (
	"os"
	"path/filepath"
	"testing"
)

// Dentro de um .app não se grava: a raiz padrão é sempre a pasta do usuário,
// e HYPHP_ROOT continua passando por cima.
func TestRaizPadraoNoMac(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(EnvRoot, "")
	if got, want := resolveRoot(), filepath.Join(home, "Library", "Application Support", "HyPHP"); got != want {
		t.Fatalf("resolveRoot() = %q, want %q", got, want)
	}
	outra := t.TempDir()
	t.Setenv(EnvRoot, outra)
	if got := resolveRoot(); got != outra {
		t.Fatalf("com HYPHP_ROOT: %q, want %q", got, outra)
	}
}

// O PATH conta como registrado só com o conteúdo exato para a pasta cli deste
// usuário: um arquivo de outra instalação (outra raiz) pede a senha de novo.
func TestPathsDRegistrado(t *testing.T) {
	t.Setenv(EnvRoot, t.TempDir())
	PathsDFile = filepath.Join(t.TempDir(), "hyphp")
	t.Cleanup(func() { PathsDFile = "/etc/paths.d/hyphp" })
	if PathsDRegistered() {
		t.Fatal("registrado sem arquivo")
	}
	if err := os.WriteFile(PathsDFile, []byte(PathsDContent("/Users/outro/cli")), 0o644); err != nil {
		t.Fatal(err)
	}
	if PathsDRegistered() {
		t.Fatal("registrado com a pasta de outra raiz")
	}
	if err := os.WriteFile(PathsDFile, []byte(PathsDContent(Cli())), 0o644); err != nil {
		t.Fatal(err)
	}
	if !PathsDRegistered() {
		t.Fatal("não registrado com o conteúdo esperado")
	}
}
