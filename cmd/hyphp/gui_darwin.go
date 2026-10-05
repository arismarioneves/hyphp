package main

import (
	"errors"
	"os/exec"

	"hyphp/internal/i18n"
)

// startGUI abre o HyPHP pelo Launch Services, que acha o .app onde quer que
// ele esteja instalado. O bundle id é exato e não depende do nome da pasta
// do app, ao contrário do "open -a". A mensagem é a mesma do Windows, com o nome do app no
// lugar do caminho.
func startGUI() error {
	if err := exec.Command("open", "-b", "com.hyphp").Start(); err != nil {
		return errors.New(i18n.T("cli.app.startFailed", "HyPHP", err))
	}
	return nil
}
