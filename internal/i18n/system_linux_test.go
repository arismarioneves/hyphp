package i18n

import (
	"slices"
	"testing"
)

func TestIdiomaDoSistemaNaOrdemDoGettext(t *testing.T) {
	casos := []struct {
		nome                              string
		language, lcAll, lcMessages, lang string
		quer                              []string
	}{
		{"LANGUAGE vem antes e mantém a ordem", "pt_BR:en", "", "", "en_US.UTF-8", []string{"pt_BR", "en", "en_US.UTF-8"}},
		{"LC_ALL antes de LANG", "", "pt_PT.UTF-8", "", "en_US.UTF-8", []string{"pt_PT.UTF-8", "en_US.UTF-8"}},
		{"repetido sai uma vez", "", "", "pt_BR.UTF-8", "pt_BR.UTF-8", []string{"pt_BR.UTF-8"}},
		{"C e POSIX não são idioma", "", "C.UTF-8", "POSIX", "C", nil},
	}
	for _, c := range casos {
		t.Setenv("LANGUAGE", c.language)
		t.Setenv("LC_ALL", c.lcAll)
		t.Setenv("LC_MESSAGES", c.lcMessages)
		t.Setenv("LANG", c.lang)
		if got := systemLanguages(); !slices.Equal(got, c.quer) {
			t.Errorf("%s: systemLanguages() = %q; quero %q", c.nome, got, c.quer)
		}
	}
}
