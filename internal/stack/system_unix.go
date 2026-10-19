//go:build darwin || linux

package stack

import (
	"errors"

	"hyphp/internal/i18n"
	"hyphp/internal/state"
)

// mkcertMissingMessage: o mkcert vem do Homebrew, pela tela Runtimes.
func mkcertMissingMessage() string { return i18n.T("warn.mkcertMissingBrew") }

// mkcertMissingError: o texto do Windows cita bin/mkcert/mkcert.exe, que não
// existe aqui; o erro aponta para o Homebrew.
func mkcertMissingError() error { return errors.New(mkcertMissingMessage()) }

// webMissingMessage: o web server vem do Homebrew, pela tela Runtimes; não
// há pasta em bin/ para citar nem o que importar.
func webMissingMessage(name state.WebServerName) string { return i18n.T("warn.webMissingBrew", name) }
