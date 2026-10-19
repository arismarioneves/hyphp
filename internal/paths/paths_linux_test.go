package paths

import (
	"path/filepath"
	"testing"
)

// A raiz segue o XDG_DATA_HOME e cai em ~/.local/share sem ele. Valor
// relativo não vale, pela especificação: conta como ausente.
func TestRaizPadraoNoLinuxSegueOXDG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(EnvRoot, "")

	dados := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dados)
	if got, want := resolveRoot(), filepath.Join(dados, "HyPHP"); got != want {
		t.Fatalf("com XDG_DATA_HOME: %q, want %q", got, want)
	}

	for _, v := range []string{"", "relativo/dados"} {
		t.Setenv("XDG_DATA_HOME", v)
		if got, want := resolveRoot(), filepath.Join(home, ".local", "share", "HyPHP"); got != want {
			t.Fatalf("XDG_DATA_HOME=%q: %q, want %q", v, got, want)
		}
	}

	outra := t.TempDir()
	t.Setenv(EnvRoot, outra)
	if got := resolveRoot(); got != outra {
		t.Fatalf("com HYPHP_ROOT: %q, want %q", got, outra)
	}
}
