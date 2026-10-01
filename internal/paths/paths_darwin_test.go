package paths

import (
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
