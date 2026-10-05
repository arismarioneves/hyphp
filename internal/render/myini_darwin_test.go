package render

import (
	"strings"
	"testing"
)

// No macOS o my.ini não tem log-error (o stderr do mysqld vai para o log do
// supervisor), tem o socket do HyPHP em [mysqld] e [client] e sobe o limite
// de arquivos abertos, que o launchd deixa em 256 para apps de GUI. O caminho
// tem espaço (Application Support): sai entre aspas.
func TestRenderMyIniGoldenNoMac(t *testing.T) {
	got := RenderMyIni(3306, "/opt/homebrew/opt/mysql@8.4", "/Users/dev/Library/Application Support/HyPHP/var/mysql-data",
		"/Users/dev/Library/Application Support/HyPHP/log", "/Users/dev/Library/Application Support/HyPHP/var/run/mysql.sock", true, false)
	checkGolden(t, "darwin/my.ini.golden", got)
}

// O MariaDB do Homebrew recebe o mesmo socket e também fica sem log-error e
// sem mysqlx.
func TestRenderMyIniMariaDBNoMac(t *testing.T) {
	sock := "/Users/dev/Library/Application Support/HyPHP/var/run/mysql.sock"
	got := string(RenderMyIni(3306, "/opt/homebrew/opt/mariadb@11.4", "/Users/dev/hy/var/mariadb-data", "/Users/dev/hy/log", sock, true, true))
	if strings.Contains(got, "log-error") || strings.Contains(got, "mysqlx") {
		t.Fatalf("log-error/mysqlx não deviam sair no MariaDB do Mac:\n%s", got)
	}
	if n := strings.Count(got, "\nsocket = \""+sock+"\"\n"); n != 2 {
		t.Fatalf("socket saiu %d vezes, want 2 ([mysqld] e [client]):\n%s", n, got)
	}
	if !strings.Contains(got, "\nopen_files_limit = 4096\n") {
		t.Fatalf("faltou open_files_limit:\n%s", got)
	}
}
