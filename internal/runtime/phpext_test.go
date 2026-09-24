package runtime

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListExtensions(t *testing.T) {
	dir := t.TempDir()
	ext := filepath.Join(dir, "ext")
	for _, f := range []string{"php_curl.dll", "php_pdo_mysql.dll", "php_zip.dll", "libssh2.dll", "readme.txt"} {
		mustWrite(t, filepath.Join(ext, f), "")
	}
	mustMkdir(t, filepath.Join(ext, "php_subdir.dll")) // diretório com nome de dll: ignorado

	got, err := ListExtensions(Installed{Kind: PHP, Dir: dir}, []string{"pdo_mysql", "inexistente"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Extension{
		{Name: "curl", File: "php_curl.dll", Enabled: false},
		{Name: "pdo_mysql", File: "php_pdo_mysql.dll", Enabled: true},
		{Name: "zip", File: "php_zip.dll", Enabled: false},
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
