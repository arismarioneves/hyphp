package main

import (
	"errors"
	"os/exec"
	"syscall"

	"hyphp/internal/i18n"
)

// appExe é onde o pacote .deb instala o app.
const appExe = "/usr/lib/hyphp/hyphp"

// startGUI abre o app numa sessão própria (Setsid): fechar o terminal da CLI
// não pode levar o app junto.
func startGUI() error {
	cmd := exec.Command(appExe)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return errors.New(i18n.T("cli.app.startFailed", appExe, err))
	}
	return cmd.Process.Release()
}
