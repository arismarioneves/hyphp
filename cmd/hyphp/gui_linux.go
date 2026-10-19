package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"hyphp/internal/i18n"
)

// appBeside é o app ao lado da pasta cli/ da CLI (bin/hyphp no build local).
func appBeside(cliExe string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(cliExe)), "hyphp")
}

// startGUI abre o app numa sessão própria (Setsid): fechar o terminal da CLI
// não pode levar o app junto. O os.Executable resolve um link para a CLI, então
// o app é achado mesmo com a CLI chamada por um link no PATH.
func startGUI() error {
	self, err := os.Executable()
	if err != nil {
		return errors.New(i18n.T("cli.app.startFailed", "HyPHP", err))
	}
	app := appBeside(self)
	cmd := exec.Command(app)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return errors.New(i18n.T("cli.app.startFailed", app, err))
	}
	return cmd.Process.Release()
}
