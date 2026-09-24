//go:build windows

package supervisor

import "testing"

// Os bytes abaixo são os que o mysqld realmente emitiu no smoke da UI: a
// mensagem de erro do Windows em CP-1252. Tratados como UTF-8 viravam
// "N�o foi poss�vel encontrar o m�dulo" na tela de Logs.
func TestDecodeLineConverteANSI(t *testing.T) {
	ansi := []byte{
		'N', 0xE3, 'o', ' ', 'f', 'o', 'i', ' ',
		'p', 'o', 's', 's', 0xED, 'v', 'e', 'l', ' ',
		'e', 'n', 'c', 'o', 'n', 't', 'r', 'a', 'r', ' ',
		'o', ' ', 'm', 0xF3, 'd', 'u', 'l', 'o',
	}
	const want = "Não foi possível encontrar o módulo"

	if got := decodeLine(ansi); got != want {
		t.Errorf("decodeLine = %q, quero %q", got, want)
	}
}

// Quem já emite UTF-8 não pode ser reinterpretado: passar "não" por CP-1252
// produziria "nÃ£o".
func TestDecodeLinePreservaUTF8(t *testing.T) {
	for _, s := range []string{
		"ready for connections",
		"não foi possível",
		"日本語",
		"",
	} {
		if got := decodeLine([]byte(s)); got != s {
			t.Errorf("decodeLine(%q) = %q, quero intacto", s, got)
		}
	}
}
