package render

import "path/filepath"

// MySQLSocketPath é o socket Unix do mysqld/mariadbd do HyPHP: <var>/run/mysql.sock.
// Fonte única do caminho: o php.ini (mysqli/pdo_mysql/mysql.default_socket) e
// o my.ini ([mysqld] e [client] socket) TÊM de usar este helper (via
// stack.mysqlSocket); se um lado montar o caminho por conta própria e
// divergir, host=localhost no PHP cai num socket que ninguém escuta.
func MySQLSocketPath(varDir string) string {
	return filepath.Join(varDir, "run", "mysql.sock")
}
