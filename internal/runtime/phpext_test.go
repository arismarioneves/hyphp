package runtime

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

// Os módulos têm o nome de ExtFile (php_<n>.dll no Windows, <n>.so no macOS);
// libssh2.dll e readme.txt não são módulo em nenhum dos dois.
func TestListExtensions(t *testing.T) {
	dir := t.TempDir()
	ext := filepath.Join(dir, "ext")
	for _, f := range []string{ExtFile("curl"), ExtFile("pdo_mysql"), ExtFile("zip"), "libssh2.dll", "readme.txt"} {
		mustWrite(t, filepath.Join(ext, f), "")
	}
	mustMkdir(t, filepath.Join(ext, ExtFile("subdir"))) // diretório com nome de módulo: ignorado

	got, err := ListExtensions(Installed{Kind: PHP, Dir: dir, ExtDir: ext}, []string{"pdo_mysql", "inexistente"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []Extension{
		{Name: "curl", File: ExtFile("curl"), Enabled: false},
		{Name: "pdo_mysql", File: ExtFile("pdo_mysql"), Enabled: true},
		{Name: "zip", File: ExtFile("zip"), Enabled: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListExtensions =\n%+v\nwant\n%+v", got, want)
	}
}

// Módulo compilado no binário aparece ligado e marcado como embutido mesmo sem
// arquivo; o que tem arquivo na pasta segue alternável. Um embutido que também
// tenha arquivo continua embutido: carregar o arquivo de novo só gera o
// warning "already loaded".
func TestListaMarcaBuiltins(t *testing.T) {
	dir := t.TempDir()
	ext := filepath.Join(dir, "ext")
	for _, f := range []string{ExtFile("curl"), ExtFile("pdo_mysql"), ExtFile("zip")} {
		mustWrite(t, filepath.Join(ext, f), "")
	}

	got, err := ListExtensions(Installed{Kind: PHP, Dir: dir, ExtDir: ext}, []string{"pdo_mysql"}, []string{"core", "zip", "date", "opcache"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Extension{
		{Name: "curl", File: ExtFile("curl")},
		{Name: "pdo_mysql", File: ExtFile("pdo_mysql"), Enabled: true},
		{Name: "core", Enabled: true, Builtin: true},
		{Name: "date", Enabled: true, Builtin: true},
		{Name: "opcache", Enabled: true, Builtin: true},
		{Name: "zip", Enabled: true, Builtin: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListExtensions =\n%+v\nwant\n%+v", got, want)
	}
}

// Saída real de `php -n -m` (8.5, Homebrew, encurtada): os cabeçalhos não são
// módulos, os nomes saem em minúsculas como os de extension=, e o "Zend
// OPcache", que aparece nas duas seções, vira um "opcache" só.
func TestParseModulos(t *testing.T) {
	out := "[PHP Modules]\r\nCore\r\nctype\r\nPDO\r\nSimpleXML\r\nZend OPcache\r\n\r\n[Zend Modules]\r\nZend OPcache\r\n\r\n"
	got := parseModules(out)
	want := []string{"core", "ctype", "pdo", "simplexml", "opcache"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseModules = %q, want %q", got, want)
	}
}

func TestDefaultExtensionsExistemNoPHP81(t *testing.T) {
	// Verifica que a lista default não referencia extensão que não vem no zip oficial
	// (nomes conferidos contra php-8.1.10-Win32-vs16-x64/ext em 2026-09-20).
	shipped := map[string]bool{}
	for _, n := range []string{"bz2", "com_dotnet", "curl", "dba", "dl_test", "enchant", "exif", "ffi", "fileinfo", "ftp", "gd", "gettext", "gmp", "imap", "intl", "ldap", "mbstring", "mysqli", "oci8_19", "odbc", "opcache", "openssl", "pdo_firebird", "pdo_mysql", "pdo_oci", "pdo_odbc", "pdo_pgsql", "pdo_sqlite", "pgsql", "shmop", "snmp", "soap", "sockets", "sodium", "sqlite3", "sysvshm", "tidy", "xsl", "zend_test"} {
		shipped[n] = true
	}
	for _, d := range DefaultExtensions {
		if !shipped[d] {
			t.Errorf("DefaultExtensions contém %q, que não existe em ext/ do PHP 8.1 oficial", d)
		}
	}
	_ = os.Getenv // mantém o import usado caso o teste acima seja removido
}

// O phpMyAdmin usa mysqli, não PDO; sem a extensão ele abre numa tela de erro
// em vez da lista de bancos. mysqli é padrão nas builds do php.net, então nada
// justifica deixá-la de fora.
func TestDefaultExtensionsTemMysqli(t *testing.T) {
	if !slices.Contains(DefaultExtensions, "mysqli") {
		t.Errorf("DefaultExtensions = %v, falta mysqli", DefaultExtensions)
	}
}
