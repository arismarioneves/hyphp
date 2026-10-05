package services

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"hyphp/internal/runtime"
	"hyphp/internal/state"
)

// extService monta um RuntimesService com um PHP 8.3 fictício: curl tem
// arquivo na pasta de módulos e zip vem compilado no binário (a consulta
// `php -n -m` trocada por uma lista fixa).
func extService(t *testing.T) (*RuntimesService, *state.State) {
	t.Helper()
	dir := t.TempDir()
	ext := filepath.Join(dir, "ext")
	if err := os.MkdirAll(ext, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ext, runtime.ExtFile("curl")), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	st := &state.State{}
	r := NewRuntimesService(RuntimesDeps{
		BinDir:      t.TempDir(),
		UpdateState: func(fn func(*state.State)) error { fn(st); return nil },
		State:       func() state.State { return *st },
	})
	r.installed = []runtime.Installed{{Kind: runtime.PHP, Version: "8.3.10", Major: "8.3", Dir: dir, ExtDir: ext}}
	r.scanned = true
	r.builtinModules = func(context.Context, runtime.Installed) ([]string, error) { return []string{"zip"}, nil }
	return r, st
}

// Embutido não tem o que ligar ou desligar: aceitar gravaria no state um nome
// que o render ignora, e a UI mostraria um toggle que não muda nada.
func TestSetExtensionRecusaEmbutida(t *testing.T) {
	r, st := extService(t)
	if err := r.SetExtension("8.3", "zip", false); err == nil {
		t.Fatal("SetExtension aceitou desligar um módulo embutido")
	}
	if st.PHPExtensions != nil {
		t.Fatalf("state mudou: %v", st.PHPExtensions)
	}
	// curl tem arquivo: segue alternável (sai da lista default da série).
	if err := r.SetExtension("8.3", "curl", false); err != nil {
		t.Fatalf("SetExtension(curl): %v", err)
	}
	if got := st.PHPExtensions["8.3"]; got == nil || slices.Contains(got, "curl") {
		t.Fatalf("PHPExtensions = %v", st.PHPExtensions)
	}
}
