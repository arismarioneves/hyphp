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

	got, err := ListExtensions(Installed{Kind: PHP, Dir: dir}, []string{"pdo_mysql", "inexistente"})
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

func TestListExtensionsSemExtDir(t *testing.T) {
	_, err := ListExtensions(Installed{Kind: PHP, Dir: t.TempDir()}, nil)
	if err == nil {
		t.Fatal("esperava erro para pasta sem ext/")
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
