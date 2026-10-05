package render

// cgiImpersonate: no macOS os workers são php-fpm, que não conhece
// fastcgi.impersonate; a diretiva só geraria ruído no php.ini.
const cgiImpersonate = false

// osManagedDirectives: no macOS o php.ini aponta os sockets do MySQL para o
// mysqld do HyPHP (MySQLSocketPath); trocá-los mandaria host=localhost para
// outro servidor, ou para o /tmp/mysql.sock de um `brew services`.
var osManagedDirectives = []string{
	"mysqli.default_socket", "pdo_mysql.default_socket", "mysql.default_socket",
}
