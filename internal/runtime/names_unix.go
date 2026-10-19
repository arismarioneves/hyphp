//go:build darwin || linux

package runtime

import (
	"path/filepath"
	"strings"

	"hyphp/internal/sysinfo"
)

// mainFile fora do Windows segue o layout do Homebrew (prefixo de cada fórmula),
// varrido pelo internal/brew. O phpMyAdmin continua reconhecido pelo
// index.php: é código PHP, não binário, e o mesmo conjunto de chaves do
// Windows evita "kind desconhecido" aqui.
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

// binKinds fora do Windows é só o phpMyAdmin, o único que o HyPHP baixa para bin/;
// os demais vêm dos kegs do Homebrew, e uma árvore bin/php perdida não pode
// aparecer ao lado deles.
var binKinds = []Kind{PhpMyAdmin}

// phpExtDir é <keg>/lib/php/<api>. O PHP_EXTENSION_DIR compilado aponta para
// o Cellar versionado; montar a partir de dir (o link opt/) com só o basename
// mantém o caminho válido depois de um `brew upgrade`.
func phpExtDir(dir, apiBase string) string { return filepath.Join(dir, "lib", "php", apiBase) }

// extDirOptional true: o PHP do Homebrew pode não ter módulo carregável
// algum (tudo estático, como o opcache a partir do 8.5), e então não há pasta.
const extDirOptional = true

// phpArch é a arquitetura do próprio HyPHP: o Homebrew instala os bottles da
// máquina, e o app roda nativo nela.
func phpArch(int) string { return sysinfo.Arch() }

// workerFile é o php-fpm: fora do Windows os workers PHP são um master FPM por
// série, não processos php-cgi soltos.
func workerFile() string { return filepath.Join("sbin", "php-fpm") }

// extFile segue o Homebrew, que instala módulos PHP como <nome>.so.
func extFile(name string) string { return name + ".so" }

// extName é o inverso de extFile; recusa ".so" sozinho, que não nomeia módulo.
func extName(file string) (string, bool) {
	name, ok := strings.CutSuffix(file, ".so")
	return name, ok && name != ""
}
