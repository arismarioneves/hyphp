package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"hyphp/internal/i18n"
)

// startGUI abre o app que mora ao lado da CLI. O erro já vem pronto para o
// usuário (com o caminho tentado), porque é ele que diz onde a instalação
// está quebrada.
func startGUI() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// A CLI mora em <instalação>\cli\hyphp.exe; o app, em <instalação>\hyphp.exe.
	gui := filepath.Join(filepath.Dir(filepath.Dir(exe)), "hyphp.exe")
	if err := exec.Command(gui).Start(); err != nil {
		return errors.New(i18n.T("cli.app.startFailed", gui, err))
	}
	return nil
}
