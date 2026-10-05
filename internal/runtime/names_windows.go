package runtime

import (
	"path/filepath"
	"strings"
)

// mainFile é o arquivo que prova que uma pasta contém o Kind, relativo à pasta
// do runtime: o executável principal para os runtimes e, no phpMyAdmin, o
// index.php — ele não tem binário, é código PHP servido pelo web server. O
// MariaDB é reconhecido pelo mariadbd.exe, e não pelo mysqld.exe que o zip dele
// também traz: uma pasta de MySQL não pode passar por MariaDB.
var mainFile = map[Kind]string{
	PHP:        "php.exe",
	Apache:     filepath.Join("bin", "httpd.exe"),
	Nginx:      "nginx.exe",
	MySQL:      filepath.Join("bin", "mysqld.exe"),
	MariaDB:    filepath.Join("bin", "mariadbd.exe"),
	Mailpit:    "mailpit.exe",
	Mkcert:     "mkcert.exe",
	PhpMyAdmin: "index.php",
}

// binKinds são os kinds que Scan procura em bin/, na ordem de saída. No
// Windows todo runtime vive em bin/<kind>/.
var binKinds = []Kind{PHP, Apache, Nginx, MySQL, MariaDB, Mailpit, Mkcert, PhpMyAdmin}

// phpExtDir é sempre <dir>/ext: o PHP_EXTENSION_DIR compilado no zip do
// php.net aponta para C:\php\ext, que não existe na máquina do usuário, então
// apiBase não serve aqui.
func phpExtDir(dir, _ string) string { return filepath.Join(dir, "ext") }

// extDirOptional false: o zip do php.net sempre traz ext/; faltar a pasta é
// instalação quebrada e deve aparecer como erro.
const extDirOptional = false

// phpArch é a arquitetura quando o nome da pasta não traz x64/x86.
func phpArch(intSize int) string {
	if intSize == 8 {
		return "x64"
	}
	return "x86"
}

// workerFile é o php-cgi.exe ao lado do php.exe: no Windows cada worker é um
// processo php-cgi servindo FastCGI.
func workerFile() string { return "php-cgi.exe" }

// extFile é o nome do módulo no ext/ do zip oficial do PHP para Windows.
func extFile(name string) string { return "php_" + name + ".dll" }

// extName é o inverso de extFile. Sem diferenciar maiúsculas porque o
// sistema de arquivos do Windows também não diferencia; o nome devolvido
// mantém a grafia do disco.
func extName(file string) (string, bool) {
	lower := strings.ToLower(file)
	if !strings.HasPrefix(lower, "php_") || !strings.HasSuffix(lower, ".dll") {
		return "", false
	}
	return file[len("php_") : len(file)-len(".dll")], true
}
