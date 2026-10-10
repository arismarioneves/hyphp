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
)

// manageHosts: no Mac os domínios .test resolvem pela regra de DNS
// (/etc/resolver/test); o /etc/hosts não é gravado nem conferido.
func manageHosts() bool { return false }

// mkcertMissingMessage: no Mac o mkcert vem do Homebrew, pela tela Runtimes.
func mkcertMissingMessage() string { return i18n.T("warn.mkcertMissingBrew") }

// mkcertMissingError: o texto do Windows cita bin/mkcert/mkcert.exe, que não
// existe no Mac; aqui o erro aponta para o Homebrew.
func mkcertMissingError() error { return errors.New(mkcertMissingMessage()) }

// trustCA no Mac em dois passos. O mkcert cria a CA como o usuário: como
// root, a chave nasceria com dono root e o app não conseguiria emitir os
// certificados dos sites. Só a confiança no keychain do sistema pede senha.
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

// SystemChanges lê, sem senha, o que o HyPHP gravou no sistema. Erro ao
// conferir a CA conta como não instalada: não há o que remover que se saiba.
// Path é só a existência do /etc/paths.d/hyphp (spec §5): um arquivo com
// outro conteúdo, deixado por outro root, ainda tem de poder sair pela UI.
func (s *Stack) SystemChanges() SystemChanges {
	ca, _ := s.mkcert().CAInstalled()
	_, perr := os.Stat(paths.PathsDFile)
	return SystemChanges{
		Supported: true,
		DNS:       resolverRuleState() == ruleOurs,
		Path:      perr == nil,
		CA:        ca,
	}
}

// RemoveSystemChanges desfaz a regra de DNS, o PATH e a confiança na CA com
// uma senha, e apaga a pasta cli, que é do usuário. Os dados do HyPHP, a CA
// em disco e os runtimes ficam.
func (s *Stack) RemoveSystemChanges(ctx context.Context) error {
	args := []string{"uninstall"}
	mk := s.mkcert()
	// O --cert vai sempre que a CA existe em disco, não só quando o
	// CAInstalled diz que está confiada: um certificado que ficou no keychain
	// sem confiança também tem de sair, e o untrust do helper pula o SHA-1
	// que não está no keychain.
	if mk.CARoot != "" {
		cert := filepath.Join(mk.CARoot, "rootCA.pem")
		if _, err := os.Stat(cert); err == nil {
			args = append(args, "--cert", cert)
		}
	}
	helper, err := elevate.HelperPath()
	if err != nil {
		return i18n.Errorf("err.stack.helperUnavailable", err)
	}
	switch err := elevate.RunElevated(helper, args); {
	case errors.Is(err, elevate.ErrElevationDenied):
		return errors.New(i18n.T("err.stack.removeCancelled"))
	case err != nil:
		return fmt.Errorf("helper uninstall: %w", err)
	}
	if err := os.RemoveAll(paths.Cli()); err != nil {
		return fmt.Errorf("stack: apagar %s: %w", paths.Cli(), err)
	}
	// Sem isto o tlsIssuer seguiria emitindo certificados de uma CA que o
	// sistema não confia mais, e o ca-pending não voltaria até reabrir.
	s.lock()
	s.caReady = false
	s.unlock()
	s.d.Logger.Info("stack: mudanças no sistema removidas")
	_, rerr := s.Reconcile(ctx)
	return rerr
}
