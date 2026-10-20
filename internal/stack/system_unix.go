//go:build darwin || linux

package stack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"hyphp/internal/elevate"
	"hyphp/internal/i18n"
	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
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

// hostsCancelled: no Linux sem systemd-resolved a senha vem do polkit, não do
// UAC.
func hostsCancelled(change string) error { return i18n.Errorf("err.stack.hostsNotUpdated", change) }

// trustCA em dois passos. O mkcert cria a CA como o usuário: como root, a
// chave nasceria com dono root e o app não conseguiria emitir os certificados
// dos sites. Só a confiança do sistema (o keychain no Mac, as âncoras no
// Linux) pede senha.
func (s *Stack) trustCA(mk netcfg.Mkcert) error {
	if err := mk.CreateCA(); err != nil {
		return i18n.Errorf("err.stack.caInstallFailed", err)
	}
	helper, herr := elevate.HelperPath()
	if herr != nil {
		return i18n.Errorf("err.stack.helperUnavailable", herr)
	}
	switch err := elevate.RunElevated(helper, []string{"ca-trust", "--cert", filepath.Join(mk.CARoot, "rootCA.pem")}); {
	case errors.Is(err, elevate.ErrElevationDenied):
		return i18n.Errorf("err.stack.caCancelled")
	case err != nil:
		return i18n.Errorf("err.stack.caTrustFailed", err)
	}
	return nil
}

// runUninstall roda o "uninstall" do helper com a senha do usuário.
func runUninstall(args []string) error {
	helper, err := elevate.HelperPath()
	if err != nil {
		return i18n.Errorf("err.stack.helperUnavailable", err)
	}
	switch err := elevate.RunElevated(helper, append([]string{"uninstall"}, args...)); {
	case errors.Is(err, elevate.ErrElevationDenied):
		return errors.New(i18n.T("err.stack.removeCancelled"))
	case err != nil:
		return fmt.Errorf("helper uninstall: %w", err)
	}
	return nil
}

// afterSystemRemoval termina o "Remover do sistema" depois do helper: apaga a
// pasta cli, que é do usuário, e reconcilia. Os dados do HyPHP, a CA em disco
// e os runtimes ficam.
func (s *Stack) afterSystemRemoval(ctx context.Context) error {
	if err := os.RemoveAll(paths.Cli()); err != nil {
		return fmt.Errorf("stack: apagar %s: %w", paths.Cli(), err)
	}
	// Sem isto o tlsIssuer seguiria emitindo certificados de uma CA que o
	// sistema não confia mais, e o ca-pending não voltaria até reabrir.
	s.lock()
	s.caReady = false
	s.unlock()
	s.d.Logger.Info("stack: mudanças no sistema removidas")
	_, err := s.Reconcile(ctx)
	return err
}
