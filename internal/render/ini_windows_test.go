package render

import (
	"bytes"
	"path/filepath"
	"testing"

	"hyphp/internal/runtime"
)

// O golden foi gerado no Windows: traz php_opcache.dll e o
// fastcgi.impersonate do php-cgi, que o php.ini do macOS não tem.
func TestRenderPHPIniGolden(t *testing.T) {
	inst := fakePHP(t, "8.1", "8.1.10", shippedIn81)
	got := RenderPHPIni(inst, runtime.DefaultExtensions, "C:/hyphp/var/tmp/php/8.1", "C:/hyphp/log", 1025, "", "", nil)
	got = bytes.ReplaceAll(got, []byte(filepath.ToSlash(inst.Dir)), []byte("__PHPDIR__"))
	checkGolden(t, "php-8.1.ini.golden", got)
}

// No Windows o my.ini continua o de sempre: log-error no arquivo (o --console
// dos args tem precedência) e sem socket, porque o mysqld.exe não usa socket
// Unix.
func TestRenderMyIniGolden(t *testing.T) {
	got := RenderMyIni(3306, "C:/hyphp/bin/mysql/mysql-8.0.30-winx64", "C:/hyphp/var/mysql", "C:/hyphp/log", "", true, false)
	checkGolden(t, "my.ini.golden", got)
}

// O MariaDB recusa opção desconhecida: com `mysqlx` no arquivo ele nem sobe.
// O resto (skip-log-bin, bind-address com dois endereços) foi conferido num
// MariaDB 10.11.19 e num 12.3.3 reais.
func TestRenderMyIniMariaDBGolden(t *testing.T) {
	got := RenderMyIni(3306, "C:/hyphp/bin/mariadb/mariadb-11.4.13-winx64", "C:/hyphp/var/mariadb-data", "C:/hyphp/log", "", true, true)
	checkGolden(t, "my.ini.mariadb.golden", got)
}

// Os *.default_socket só valem no macOS; no Windows o php-cgi não usa socket
// Unix e o painel volta a aceitá-los do usuário, como antes do suporte ao Mac.
func TestSocketsDoMySQLNaoSaoGerenciadosNoWindows(t *testing.T) {
	for _, n := range []string{"mysqli.default_socket", "pdo_mysql.default_socket", "mysql.default_socket"} {
		if IsManagedIniDirective(n) {
			t.Errorf("%s não devia ser gerenciada no Windows", n)
		}
	}
}
