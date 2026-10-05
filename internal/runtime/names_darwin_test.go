package runtime

import (
	"path/filepath"
	"testing"
)

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

// No Homebrew os módulos ficam em <keg>/lib/php/<api>. Só o basename do
// PHP_EXTENSION_DIR entra: o valor compilado aponta para o Cellar versionado,
// e montar a partir do opt/ mantém o caminho válido depois de um upgrade.
func TestExtDirPorSO(t *testing.T) {
	dir := "/opt/homebrew/opt/php@8.3"
	if got, want := phpExtDir(dir, "20230831"), filepath.Join(dir, "lib", "php", "20230831"); got != want {
		t.Fatalf("phpExtDir = %q, want %q", got, want)
	}
}

// O tap shivammathur/php só publica builds Apple Silicon.
func TestArquiteturaDoPHPNoMac(t *testing.T) {
	if got := phpArch(8); got != "arm64" {
		t.Fatalf("phpArch(8) = %q", got)
	}
}
