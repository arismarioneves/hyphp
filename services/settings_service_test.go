package services

import (
	"path/filepath"
	"slices"
	"testing"

	"hyphp/internal/stack"
	"hyphp/internal/state"
)

// A tela Configurações manda o state inteiro de quando o rascunho foi
// carregado. Uma pasta adicionada, uma extensão ligada e uma troca de web
// server feitas depois disso não podem ser desfeitas quando o usuário salva
// outro campo.
func TestSetPreservaCamposDeOutrasTelas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	atual := state.Default()
	atual.Roots = []string{`C:\DEV`, `D:\novos`}
	atual.PHPExtensions = map[string][]string{"8.3": {"intl"}}
	atual.WebServer = state.Nginx

	velho := atual
	velho.Roots = []string{`C:\DEV`}
	velho.PHPExtensions = nil
	velho.Terminal = "wt.exe"
	velho.WebServer = state.Apache

	vivo := atual
	s := NewSettingsService(stack.New(stack.Deps{State: &vivo, StatePath: path}), func(string, any) {}, nil, nil)
	s.applyAutostart = func(bool) error { return nil }
	if err := s.Set(velho); err != nil {
		t.Fatal(err)
	}

	got, err := state.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Roots, atual.Roots) {
		t.Errorf("Roots = %v, want %v", got.Roots, atual.Roots)
	}
	if !slices.Equal(got.PHPExtensions["8.3"], []string{"intl"}) {
		t.Errorf("PHPExtensions = %v, want 8.3: [intl]", got.PHPExtensions)
	}
	if got.Terminal != "wt.exe" {
		t.Errorf("Terminal = %q, want wt.exe", got.Terminal)
	}
	if got.WebServer != state.Nginx {
		t.Errorf("WebServer = %q, want nginx", got.WebServer)
	}
}
