package autostart

import "testing"

func TestHiddenSoNoLoginComAOpcaoLigada(t *testing.T) {
	casos := []struct {
		nome  string
		args  []string
		opcao bool
		quer  bool
	}{
		{"login com a opção ligada", []string{Flag}, true, true},
		{"login com a opção desligada", []string{Flag}, false, false},
		{"aberto pelo usuário com a opção ligada", nil, true, false},
		{"aberto pelo usuário com outro argumento", []string{"--outro"}, true, false},
	}
	for _, c := range casos {
		if got := Hidden(c.args, c.opcao); got != c.quer {
			t.Errorf("%s: Hidden(%q, %v) = %v; quero %v", c.nome, c.args, c.opcao, got, c.quer)
		}
	}
}
