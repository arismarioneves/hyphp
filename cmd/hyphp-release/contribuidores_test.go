package main

import (
	"slices"
	"testing"
)

func respostaGH(saida string) func(args ...string) ([]byte, error) {
	return func(...string) ([]byte, error) { return []byte(saida), nil }
}

func TestContribuidoresTiraBotsEMantemAOrdemEntrePaginas(t *testing.T) {
	saida := `[
		[{"login":"ana","type":"User"},{"login":"dependabot[bot]","type":"Bot"},{"login":"bia","type":"User"}],
		[{"login":"github-actions[bot]","type":"Bot"},{"login":"caio","type":"User"},{"login":"renovate[bot]","type":"User"}]
	]`
	got, err := contribuidores(respostaGH(saida), "dono/repo")
	if err != nil {
		t.Fatal(err)
	}
	if quer := []string{"ana", "bia", "caio"}; !slices.Equal(got, quer) {
		t.Errorf("contribuidores = %q; quero %q", got, quer)
	}
}

// Gravar uma lista vazia publicaria o app sem ninguém nos créditos.
func TestContribuidoresSoComBotsEhErro(t *testing.T) {
	if _, err := contribuidores(respostaGH(`[[{"login":"dependabot[bot]","type":"Bot"}]]`), "dono/repo"); err == nil {
		t.Fatal("lista só com bots não deu erro")
	}
}
