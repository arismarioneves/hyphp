//go:build darwin || linux

package stack

import (
	"errors"

	"hyphp/internal/i18n"
)

// mkcertMissingMessage: o mkcert vem do Homebrew, pela tela Runtimes.
func mkcertMissingMessage() string { return i18n.T("warn.mkcertMissingBrew") }

// mkcertMissingError: o texto do Windows cita bin/mkcert/mkcert.exe, que não
// existe aqui; o erro aponta para o Homebrew.
func mkcertMissingError() error { return errors.New(mkcertMissingMessage()) }
