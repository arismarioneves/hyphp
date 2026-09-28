package render

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hyphp/internal/runtime"
)

// fakePHP monta um runtime.Installed com um ext/ povoado, que é o que
// RenderPHPIni consulta para decidir quais `extension=` emitir.
func fakePHP(t *testing.T, major, version string, exts []string) runtime.Installed {
	t.Helper()
	dir := t.TempDir()
	extDir := filepath.Join(dir, "ext")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, e := range exts {
		if err := os.WriteFile(filepath.Join(extDir, "php_"+e+".dll"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return runtime.Installed{
		Kind: runtime.PHP, Version: version, Major: major, Dir: dir,
		Exe: filepath.Join(dir, "php.exe"), CGIExe: filepath.Join(dir, "php-cgi.exe"),
	}
}

// shippedIn81 é o ext/ real do PHP 8.1.10 para Windows no que toca a
// DefaultExtensions: `zip` é estático e NÃO tem DLL.
var shippedIn81 = []string{
	"curl", "exif", "fileinfo", "gd", "intl", "mbstring", "opcache",
	"openssl", "pdo_mysql", "pdo_sqlite", "soap", "sodium", "sqlite3",
}

func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler golden %s: %v", path, err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), got) {
		os.WriteFile(path+".got", got, 0o644)
		t.Fatalf("saída difere de %s; gravei %s.got para diff", path, path)
	}
}

func TestRenderPHPIniGolden(t *testing.T) {
	inst := fakePHP(t, "8.1", "8.1.10", shippedIn81)
	got := RenderPHPIni(inst, runtime.DefaultExtensions, "C:/hyphp/var/tmp/php/8.1", "C:/hyphp/log", 1025)
	got = bytes.ReplaceAll(got, []byte(filepath.ToSlash(inst.Dir)), []byte("__PHPDIR__"))
	checkGolden(t, "php-8.1.ini.golden", got)
}

func TestRenderPHPIniDeterministico(t *testing.T) {
	inst := fakePHP(t, "8.1", "8.1.10", shippedIn81)
	a := RenderPHPIni(inst, []string{"curl", "gd", "intl"}, "C:/tmp", "C:/log", 1025)
	b := RenderPHPIni(inst, []string{"curl", "gd", "intl"}, "C:/tmp", "C:/log", 1025)
	if !bytes.Equal(a, b) {
		t.Fatal("duas chamadas iguais produziram bytes diferentes")
	}
	// Ordem de entrada e duplicatas não podem mudar a saída.
	c := RenderPHPIni(inst, []string{"intl", "curl", "gd", "curl"}, "C:/tmp", "C:/log", 1025)
	if !bytes.Equal(a, c) {
		t.Fatalf("saída depende da ordem de entrada:\n%s\n---\n%s", a, c)
	}
	if !strings.Contains(string(a), "extension=curl\nextension=gd\nextension=intl\n") {
		t.Fatalf("extensões não saíram ordenadas:\n%s", a)
	}
}

func TestRenderPHPIniIgnoraExtensaoSemDLL(t *testing.T) {
	inst := fakePHP(t, "8.1", "8.1.10", shippedIn81)
	out := string(RenderPHPIni(inst, runtime.DefaultExtensions, "C:/tmp", "C:/log", 1025))
	if strings.Contains(out, "extension=zip") {
		t.Fatal("emitiu extension=zip sem php_zip.dll; isso vira warning no corpo da resposta")
	}
	if !strings.Contains(out, "zend_extension=php_opcache.dll") {
		t.Fatal("opcache existe em ext/ e devia sair como zend_extension")
	}
	if strings.Contains(out, "extension=opcache") {
		t.Fatal("opcache é zend_extension, nunca extension")
	}
}

func TestRenderMyIniGolden(t *testing.T) {
	got := RenderMyIni(3306, "C:/hyphp/bin/mysql/mysql-8.0.30-winx64", "C:/hyphp/var/mysql", "C:/hyphp/log", true)
	checkGolden(t, "my.ini.golden", got)
}

// Sem ::1 na máquina o mysqld aborta ao não conseguir escutar num dos
// endereços da lista; aí só o IPv4 pode ir para o bind-address.
func TestRenderMyIniSemIPv6(t *testing.T) {
	got := string(RenderMyIni(3306, "C:/m", "C:/d", "C:/l", false))
	if !strings.Contains(got, "\nbind-address = 127.0.0.1\n") {
		t.Fatalf("bind-address devia ser só 127.0.0.1:\n%s", got)
	}
}
