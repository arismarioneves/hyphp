//go:build darwin || linux

package render

import (
	"strings"
	"testing"
)

// Fora do Windows o my.ini não tem log-error (o stderr do mysqld vai para o log
// do supervisor), tem o socket do HyPHP em [mysqld] e [client] e sobe o limite
// de arquivos abertos, que app de GUI pode herdar baixo. O caminho tem espaço
// (Application Support): sai entre aspas.
func TestRenderMyIniGoldenForaDoWindows(t *testing.T) {
	got := RenderMyIni(3306, "/opt/homebrew/opt/mysql@8.4", "/Users/dev/Library/Application Support/HyPHP/var/mysql-data",
		"/Users/dev/Library/Application Support/HyPHP/log", "/Users/dev/Library/Application Support/HyPHP/var/run/mysql.sock", true, false)
	checkGolden(t, "unix/my.ini.golden", got)
}

// O MariaDB do Homebrew recebe o mesmo socket e também fica sem log-error e
// sem mysqlx.
func TestRenderMyIniMariaDBForaDoWindows(t *testing.T) {
	sock := "/Users/dev/Library/Application Support/HyPHP/var/run/mysql.sock"
	got := string(RenderMyIni(3306, "/opt/homebrew/opt/mariadb@11.4", "/Users/dev/hy/var/mariadb-data", "/Users/dev/hy/log", sock, true, true))
	if strings.Contains(got, "log-error") || strings.Contains(got, "mysqlx") {
		t.Fatalf("log-error/mysqlx não deviam sair no MariaDB do Homebrew:\n%s", got)
	}
	if n := strings.Count(got, "\nsocket = \""+sock+"\"\n"); n != 2 {
		t.Fatalf("socket saiu %d vezes, want 2 ([mysqld] e [client]):\n%s", n, got)
	}
	if !strings.Contains(got, "\nopen_files_limit = 4096\n") {
		t.Fatalf("faltou open_files_limit:\n%s", got)
	}
}
