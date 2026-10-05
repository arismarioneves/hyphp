package runtime

import (
	"path/filepath"
	"strings"
)

// mainFile no macOS segue o layout do Homebrew (prefixo de cada fórmula),
// varrido pelo internal/brew. O phpMyAdmin continua reconhecido pelo
// index.php: é código PHP, não binário, e o mesmo conjunto de chaves do
// Windows evita "kind desconhecido" no Mac.
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

// binKinds no macOS é só o phpMyAdmin, o único que o HyPHP baixa para bin/;
// os demais vêm dos kegs do Homebrew, e uma árvore bin/php perdida não pode
// aparecer ao lado deles.
var binKinds = []Kind{PhpMyAdmin}

// phpExtDir é <keg>/lib/php/<api>. O PHP_EXTENSION_DIR compilado aponta para
// o Cellar versionado; montar a partir de dir (o link opt/) com só o basename
// mantém o caminho válido depois de um `brew upgrade`.
func phpExtDir(dir, apiBase string) string { return filepath.Join(dir, "lib", "php", apiBase) }

// phpArch é arm64: o tap shivammathur/php só publica builds Apple Silicon.
func phpArch(int) string { return "arm64" }

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
