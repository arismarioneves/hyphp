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
