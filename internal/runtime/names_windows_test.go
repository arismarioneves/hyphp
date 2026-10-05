package runtime

import (
	"path/filepath"
	"testing"
)

// No zip do Windows os módulos ficam em <pasta>/ext, qualquer que seja o
// PHP_EXTENSION_DIR compilado (o do php.net aponta para C:\php\ext, que não
// existe na máquina do usuário).
func TestExtDirPorSO(t *testing.T) {
	dir := filepath.FromSlash("C:/HyPHP/bin/php/php-8.3.10-nts-Win32-vs16-x64")
	if got, want := phpExtDir(dir, "ext"), filepath.Join(dir, "ext"); got != want {
		t.Fatalf("phpExtDir = %q, want %q", got, want)
	}
	if got, want := phpExtDir(dir, "20230831"), filepath.Join(dir, "ext"); got != want {
		t.Fatalf("phpExtDir ignora apiBase: %q, want %q", got, want)
	}
}

// Sem x64/x86 no nome da pasta, a arquitetura vem do PHP_INT_SIZE.
func TestArquiteturaDoPHPNoWindows(t *testing.T) {
	if got := phpArch(8); got != "x64" {
		t.Fatalf("phpArch(8) = %q", got)
	}
	if got := phpArch(4); got != "x86" {
		t.Fatalf("phpArch(4) = %q", got)
	}
}

// O zip do php.net sempre traz ext/: sem a pasta a build está quebrada, e o
// painel precisa mostrar o erro em vez de uma lista só de embutidos.
func TestListaSemPastaExtEhErro(t *testing.T) {
	dir := t.TempDir()
	if _, err := ListExtensions(Installed{Kind: PHP, Dir: dir, ExtDir: filepath.Join(dir, "ext")}, nil, []string{"core"}); err == nil {
		t.Fatal("ListExtensions sem ext/ devolveu nil; quer erro")
	}
}
