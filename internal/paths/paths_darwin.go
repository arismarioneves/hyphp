package paths

import (
	"os"
	"path/filepath"
)

// defaultRoot no macOS é ~/Library/Application Support/HyPHP. Ao lado do
// executável nunca: num .app assinado o bundle não pode mudar, e em
// /Applications o usuário comum nem tem escrita.
func defaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		wd, _ := os.Getwd()
		return filepath.Join(wd, "HyPHP")
	}
	return filepath.Join(home, "Library", "Application Support", "HyPHP")
}

// PathsDFile é o arquivo que o path_helper do macOS lê para montar o PATH dos
// shells de login. Variável: os testes apontam para uma pasta temporária.
var PathsDFile = "/etc/paths.d/hyphp"

// PhpBin é o atalho, dentro de Cli(), para a pasta bin do PHP padrão.
const PhpBin = "php-bin"

// PathsDContent é o /etc/paths.d/hyphp esperado para a pasta cli: ela mesma
// (o hyphp) e o php-bin. Sem o atalho php-bin a segunda linha aponta para
// nada, e o shell a ignora.
func PathsDContent(cliDir string) string {
	return cliDir + "\n" + filepath.Join(cliDir, PhpBin) + "\n"
}

// PathsDRegistered diz se o /etc/paths.d/hyphp já tem a pasta cli deste
// usuário. Ler não pede senha.
func PathsDRegistered() bool {
	raw, err := os.ReadFile(PathsDFile)
	return err == nil && string(raw) == PathsDContent(Cli())
}
