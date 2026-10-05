package render

import "testing"

// No macOS o php.ini aponta os *.default_socket para o socket do mysqld do
// HyPHP; o usuário trocá-los mandaria host=localhost para outro servidor.
func TestSocketsDoMySQLSaoGerenciadosNoMac(t *testing.T) {
	for _, n := range []string{"mysqli.default_socket", "PDO_MYSQL.default_socket", "mysql.default_socket"} {
		if !IsManagedIniDirective(n) {
			t.Errorf("%s devia ser gerenciada no macOS", n)
		}
	}
}
