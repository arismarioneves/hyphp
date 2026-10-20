package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// defaultRoot no Linux é $XDG_DATA_HOME/HyPHP, com ~/.local/share como padrão
// da especificação XDG. Ao lado do executável nunca: instalado, o app fica
// numa pasta do sistema, onde o usuário não grava.
func defaultRoot() string {
	if d := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(d) {
		return filepath.Join(d, "HyPHP")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		wd, _ := os.Getwd()
		return filepath.Join(wd, "HyPHP")
	}
	return filepath.Join(home, ".local", "share", "HyPHP")
}

// ProfileDFile é o trecho de shell que o /etc/profile de todo shell de login
// lê para montar o PATH, o equivalente do /etc/paths.d do Mac. Variável: os
// testes apontam para uma pasta temporária.
var ProfileDFile = "/etc/profile.d/hyphp.sh"

// ProfileDUnsafe são os caracteres que o caminho da pasta cli não pode ter: o
// arquivo é shell, e dentro das aspas duplas eles seriam interpretados.
const ProfileDUnsafe = "\"$`\\\n\r"

// ProfileDContent é o /etc/profile.d/hyphp.sh para a pasta cli do usuário uid:
// ela (o hyphp) e o php-bin no fim do PATH, como o path_helper do Mac faz com
// o paths.d. Só o dono da pasta ganha a entrada; os outros usuários da máquina
// não passam a rodar programas da pasta dele. O caminho não pode ter
// ProfileDUnsafe.
func ProfileDContent(cliDir string, uid int) string {
	bin := filepath.Join(cliDir, PhpBin)
	var b strings.Builder
	b.WriteString("# hyphp\n")
	fmt.Fprintf(&b, "if [ \"$(id -u)\" = \"%d\" ]; then\n", uid)
	b.WriteString("\tcase \":$PATH:\" in\n")
	fmt.Fprintf(&b, "\t*\":%s:\"*) ;;\n", cliDir)
	fmt.Fprintf(&b, "\t*) PATH=\"$PATH:%s:%s\" ;;\n", cliDir, bin)
	b.WriteString("\tesac\n")
	b.WriteString("\texport PATH\n")
	b.WriteString("fi\n")
	return b.String()
}

// ProfileDRegistered diz se o /etc/profile.d/hyphp.sh já tem a pasta cli
// deste usuário. Ler não pede senha.
func ProfileDRegistered() bool {
	raw, err := os.ReadFile(ProfileDFile)
	return err == nil && string(raw) == ProfileDContent(Cli(), os.Getuid())
}
