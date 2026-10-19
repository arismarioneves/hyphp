package paths

import (
	"os"
	"path/filepath"
)

// defaultRoot no Linux é $XDG_DATA_HOME/HyPHP, com ~/.local/share como padrão
// da especificação XDG. Ao lado do executável nunca: o pacote instala em
// /usr/lib/hyphp, onde o usuário não grava.
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
