package stack

import (
	"context"
	"os"
	"path/filepath"

	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
	"hyphp/internal/state"
)

// manageHosts: no Mac os domínios .test resolvem pela regra de DNS
// (/etc/resolver/test); o /etc/hosts não é gravado nem conferido.
func manageHosts() bool { return false }

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
	var args []string
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
	if err := runUninstall(args); err != nil {
		return err
	}
	return s.afterSystemRemoval(ctx)
}

// portsBelowLimit: o macOS abre as portas baixas sem root.
func portsBelowLimit(state.State) []Warning { return nil }

// ApplyPorts: o macOS abre as portas baixas sem senha, então não há limite a
// liberar.
func (s *Stack) ApplyPorts(context.Context) error { return nil }

// syncBrowserTrust: no Mac a CA vai para os navegadores só no InstallCA.
func (s *Stack) syncBrowserTrust(netcfg.Mkcert) {}
