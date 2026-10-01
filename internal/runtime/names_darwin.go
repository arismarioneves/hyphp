package runtime

import (
	"path/filepath"
	"strings"
)

// mainFile no macOS segue o layout do Homebrew (prefixo de cada fórmula). A
// M1 aponta a varredura para esses prefixos; aqui fica só o nome. O
// phpMyAdmin continua reconhecido pelo index.php: é código PHP, não binário,
// e o mesmo conjunto de chaves do Windows evita "kind desconhecido" no Mac.
var mainFile = map[Kind]string{
	PHP:        filepath.Join("bin", "php"),
	Apache:     filepath.Join("bin", "httpd"),
	Nginx:      filepath.Join("bin", "nginx"),
	MySQL:      filepath.Join("bin", "mysqld"),
	MariaDB:    filepath.Join("bin", "mariadbd"),
	Mailpit:    filepath.Join("bin", "mailpit"),
	Mkcert:     filepath.Join("bin", "mkcert"),
	PhpMyAdmin: "index.php",
}

// workerFile é o php-fpm: no macOS os workers PHP são um master FPM por
// série, não processos php-cgi soltos.
func workerFile() string { return filepath.Join("sbin", "php-fpm") }

// extFile segue o Homebrew, que instala módulos PHP como <nome>.so.
func extFile(name string) string { return name + ".so" }

// extName é o inverso de extFile; recusa ".so" sozinho, que não nomeia módulo.
func extName(file string) (string, bool) {
	name, ok := strings.CutSuffix(file, ".so")
	return name, ok && name != ""
}
