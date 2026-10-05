package services

import (
	"os"
	"path/filepath"
)

// defaultRootDir devolve ~/Code (criando se preciso). Fica fora de
// Desktop/Documents/Downloads porque essas pastas são protegidas pelo macOS:
// a cada atualização assinada ad-hoc o sistema pediria permissão de novo, e os
// servidores (httpd/php-fpm) lendo projetos ali também disparariam prompts.
// Em qualquer erro devolve "" e o diálogo abre onde o macOS escolher.
func defaultRootDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	dir := filepath.Join(home, "Code")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	return dir
}
