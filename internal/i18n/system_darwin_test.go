package i18n

import (
	"reflect"
	"testing"
)

// O defaults imprime um array no formato OpenStep; itens simples vêm sem aspas.
func TestParseAppleLanguages(t *testing.T) {
	out := "(\n    \"pt-BR\",\n    en,\n    \"es-419\"\n)\n"
	if got, want := parseAppleLanguages(out), []string{"pt-BR", "en", "es-419"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := parseAppleLanguages("()\n"); len(got) != 0 {
		t.Fatalf("lista vazia virou %v", got)
	}
}
