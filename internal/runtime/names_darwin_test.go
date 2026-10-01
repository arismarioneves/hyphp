package runtime

import "testing"

// O Homebrew instala módulos PHP como <nome>.so; o inverso precisa recusar
// o que não é módulo, senão a lista de extensões mostra lixo da pasta.
func TestExtensaoNoMac(t *testing.T) {
	if got := extFile("intl"); got != "intl.so" {
		t.Fatalf("extFile = %q", got)
	}
	if n, ok := extName("intl.so"); !ok || n != "intl" {
		t.Fatalf("extName(intl.so) = %q, %v", n, ok)
	}
	if _, ok := extName("intl.dylib"); ok {
		t.Fatal("intl.dylib aceito como extensão")
	}
}
