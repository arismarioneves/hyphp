package services

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hyphp/internal/paths"
	"hyphp/internal/state"
)

func newTestService(t *testing.T) *AppService {
	t.Helper()
	st := state.Default()
	return NewAppService(AppDeps{
		Quit:   func() {},
		State:  &st,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
}

// OpenExternal só pode abrir http/https: aceitar file:// ou um scheme arbitrário
// transformaria um domínio vindo de hyphp.yaml em execução de caminho local.
func TestOpenExternalRecusaEsquemaNaoWeb(t *testing.T) {
	a := newTestService(t)
	tests := []struct {
		name string
		url  string
	}{
		{"file local", `file:///C:/Windows/System32/calc.exe`},
		{"sem esquema", "example.test"},
		{"esquema arbitrario", "javascript:alert(1)"},
		{"http sem host", "http://"},
		{"vazio", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := a.OpenExternal(tt.url)
			if err == nil {
				t.Fatalf("OpenExternal(%q) devia falhar", tt.url)
			}
			if !strings.Contains(err.Error(), "url invalida") {
				t.Fatalf("erro inesperado: %v", err)
			}
		})
	}
}

// OpenFolder/OpenTerminal recebem caminho vindo da UI; arquivo ou caminho
// inexistente não pode virar chamada ao Explorer com argumento arbitrário.
func TestAberturaDeDiretorioValidaCaminho(t *testing.T) {
	a := newTestService(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "arquivo.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "nao-existe")

	tests := []struct {
		name    string
		path    string
		wantSub string
	}{
		{"inexistente", missing, "diretorio inexistente"},
		{"arquivo em vez de diretorio", file, "nao e diretorio"},
	}
	for _, tt := range tests {
		t.Run("OpenFolder/"+tt.name, func(t *testing.T) {
			err := a.OpenFolder(tt.path)
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("OpenFolder(%q) = %v, queria conter %q", tt.path, err, tt.wantSub)
			}
		})
		t.Run("OpenTerminal/"+tt.name, func(t *testing.T) {
			err := a.OpenTerminal(tt.path)
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("OpenTerminal(%q) = %v, queria conter %q", tt.path, err, tt.wantSub)
			}
		})
	}

	t.Run("OpenInEditor/inexistente", func(t *testing.T) {
		err := a.OpenInEditor(missing)
		if err == nil || !strings.Contains(err.Error(), "caminho inexistente") {
			t.Fatalf("OpenInEditor(%q) = %v", missing, err)
		}
	})
}

func TestRuntimeRootSegueHYPHPROOT(t *testing.T) {
	root := t.TempDir()
	t.Setenv(paths.EnvRoot, root)
	if got := newTestService(t).RuntimeRoot(); got != root {
		t.Fatalf("RuntimeRoot() = %q, want %q", got, root)
	}
}
