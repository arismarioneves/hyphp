package stack

import (
	"path/filepath"

	"hyphp/internal/runtime"
	"hyphp/internal/sysproc"
)

// mysqldExtraArgs vêm depois do --defaults-file no servidor e no
// --initialize-insecure. --console (só existe no Windows) manda o log para o
// stdout, que o supervisor grava no log/mysql.log e o init no mysql-init.log;
// ele tem precedência sobre o log-error do my.ini.
var mysqldExtraArgs = []string{"--console"}

// mariadbInitCmd: o mariadb-install-db.exe do Windows cria o datadir com root
// sem senha, como o --initialize-insecure do MySQL. Ele grava um my.ini
// próprio dentro do datadir, que fica sem uso: o servidor sobe com
// --defaults-file.
func mariadbInitCmd(inst runtime.Installed, data string) (string, []string) {
	return filepath.Join(inst.Dir, "bin", sysproc.ExeName("mariadb-install-db")), []string{"--datadir=" + data}
}

// mysqlSocket: o mysqld do Windows não usa socket Unix; nem o php.ini nem o
// my.ini emitem socket.
func mysqlSocket(string) string { return "" }
