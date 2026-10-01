package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoot(t *testing.T) {
	custom := t.TempDir()

	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "HYPHP_ROOT definido vence", env: custom, want: custom},
		{name: "HYPHP_ROOT relativo vira absoluto", env: ".", want: mustAbs(t, ".")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvRoot, tt.env)
			if got := Root(); got != tt.want {
				t.Fatalf("Root() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSubdirsDerivamDeRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvRoot, root)

	tests := []struct {
		name string
		fn   func() string
		want string
	}{
		{"Bin", Bin, filepath.Join(root, "bin")},
		{"Etc", Etc, filepath.Join(root, "etc")},
		{"Var", Var, filepath.Join(root, "var")},
		{"Log", Log, filepath.Join(root, "log")},
		{"Run", Run, filepath.Join(root, "var", "run")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(); got != tt.want {
				t.Fatalf("%s() = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestEnsureLayout(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvRoot, root)

	if err := EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	for _, rel := range []string{"bin", "etc", "var", filepath.Join("var", "run"), "log"} {
		info, err := os.Stat(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("%s nao criado: %v", rel, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s nao e diretorio", rel)
		}
	}
	// idempotente
	if err := EnsureLayout(); err != nil {
		t.Fatalf("segunda chamada: %v", err)
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
