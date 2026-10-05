package stack

import (
	"path/filepath"

	"hyphp/internal/render"
	"hyphp/internal/runtime"
)

// mysqldExtraArgs: nada depois do --defaults-file. --console só existe no
// Windows; aqui o my.ini não tem log-error e o mysqld/mariadbd escreve no
// stderr, que o supervisor grava no log/mysql.log e o init no mysql-init.log.
var mysqldExtraArgs []string

// mariadbInitCmd: no Unix o mariadb-install-db é um script com outras opções.
//   - --no-defaults: sem isso ele lê o /opt/homebrew/etc/my.cnf e o my.cnf.d/;
//   - --basedir no keg, como no teste do próprio Homebrew;
//   - --auth-root-authentication-method=normal: desde o 10.4 o padrão é o
//     root@localhost por unix_socket com senha inválida, e o HyPHP (mysqlcli,
//     phpMyAdmin) entra como root sem senha por TCP;
//   - --skip-test-db: o banco "test" aberto a qualquer usuário não tem uso aqui.
//
// Sem --user: só é necessário quando o script roda como root.
func mariadbInitCmd(inst runtime.Installed, data string) (string, []string) {
	return filepath.Join(inst.Dir, "bin", "mariadb-install-db"), []string{
		"--no-defaults",
		"--basedir=" + inst.Dir,
		"--datadir=" + data,
		"--auth-root-authentication-method=normal",
		"--skip-test-db",
	}
}

// mysqlSocket é o socket do mysqld do HyPHP, fonte única para o php.ini
// (*.default_socket) e o my.ini ([mysqld] e [client]).
func mysqlSocket(varDir string) string { return render.MySQLSocketPath(varDir) }
