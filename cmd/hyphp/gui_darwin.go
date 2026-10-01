package main

import (
	"errors"
	"os/exec"

	"hyphp/internal/i18n"
)

// startGUI abre o HyPHP pelo Launch Services, que acha o .app onde quer que
// ele esteja instalado. A mensagem é a mesma do Windows, com o nome do app no
// lugar do caminho.
func startGUI() error {
	if err := exec.Command("open", "-a", "HyPHP").Start(); err != nil {
		return errors.New(i18n.T("cli.app.startFailed", "HyPHP", err))
	}
	return nil
}
