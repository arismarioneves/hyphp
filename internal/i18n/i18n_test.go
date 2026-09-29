package i18n

import (
	"regexp"
	"slices"
	"testing"
)

var verbRe = regexp.MustCompile(`%[-+# 0]*[0-9]*(\.[0-9]+)?[a-zA-Z%]`)

// Todo idioma tem as mesmas chaves do catálogo de referência, com os mesmos
// verbos de formatação na mesma ordem: uma tradução que perde um %s imprime
// "%!(EXTRA string=...)" na tela, e uma chave faltando mostra a chave crua.
func TestCatalogosCompletos(t *testing.T) {
	for _, l := range Supported {
		cat, ok := catalogs[l]
		if !ok {
			t.Fatalf("%s está em Supported mas não tem catálogo", l)
		}
		for key, ref := range messagesPT {
			msg, ok := cat[key]
			if !ok {
				t.Errorf("%s: falta a chave %q", l, key)
				continue
			}
			if a, b := verbRe.FindAllString(ref, -1), verbRe.FindAllString(msg, -1); !slices.Equal(a, b) {
				t.Errorf("%s: %q tem verbos %v, o pt-BR tem %v", l, key, b, a)
			}
		}
		for key := range cat {
			if _, ok := messagesPT[key]; !ok {
				t.Errorf("%s: chave %q não existe no pt-BR", l, key)
			}
		}
	}
}

func TestFromSystem(t *testing.T) {
	casos := []struct {
		langs []string
		want  Lang
	}{
		{[]string{"pt-BR"}, PT},
		{[]string{"pt-PT"}, PT},
		{[]string{"en-US", "pt-BR"}, EN},
		{[]string{"es-ES", "pt-BR"}, PT}, // espanhol não suportado: vale o próximo da lista
		{[]string{"de-DE"}, EN},
		{nil, EN},
		{[]string{"pt_BR.UTF-8"}, PT},
	}
	for _, c := range casos {
		if got := fromSystem(c.langs); got != c.want {
			t.Errorf("fromSystem(%v) = %s, quero %s", c.langs, got, c.want)
		}
	}
}

func TestResolvePreferenciaGanhaDoSistema(t *testing.T) {
	if got := Resolve("en"); got != EN {
		t.Errorf("Resolve(en) = %s", got)
	}
	if got := Resolve("pt-BR"); got != PT {
		t.Errorf("Resolve(pt-BR) = %s", got)
	}
	if Valid("xx") || !Valid("") || !Valid("en") {
		t.Error("Valid aceita/recusa errado")
	}
}

func TestTFormataEChaveAusenteApareceCrua(t *testing.T) {
	SetCurrent(EN)
	t.Cleanup(func() { SetCurrent(PT) })
	if got := T("tray.updateTo", "3.0.0"); got != "Update to 3.0.0 and restart" {
		t.Errorf("T = %q", got)
	}
	if got := T("nao.existe"); got != "nao.existe" {
		t.Errorf("chave ausente = %q", got)
	}
}
