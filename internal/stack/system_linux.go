package stack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"hyphp/internal/elevate"
	"hyphp/internal/i18n"
	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
	"hyphp/internal/state"
)

// manageHosts: com o systemd-resolved (Ubuntu, Mint), os domínios .test
// resolvem pela regra de DNS do HyPHP, como no Mac; sem ele (o Debian na
// instalação padrão), pelo /etc/hosts, como no Windows. Lido a cada chamada:
// o resolved pode ser ligado ou desligado com o app aberto.
func manageHosts() bool { return !netcfg.ResolvedStub() }

// SystemChanges lê, sem senha, o que o HyPHP gravou no sistema. Erro ao
// conferir a CA conta como não instalada: não há o que remover que se saiba.
func (s *Stack) SystemChanges() SystemChanges {
	ca, _ := s.mkcert().CAInstalled()
	_, perr := os.Stat(paths.ProfileDFile)
	return SystemChanges{
		Supported: true,
		DNS:       markedFile(netcfg.ResolvedDropInPath, netcfg.ParseResolvedDropIn),
		Path:      perr == nil,
		CA:        ca,
		Ports:     markedFile(netcfg.SysctlPortsPath, netcfg.ParseSysctlPorts),
		Hosts:     hostsBlockPresent(),
	}
}

// markedFile diz se o arquivo existe com a marca do HyPHP.
func markedFile(path string, parse func(string) (bool, int)) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	ours, _ := parse(string(raw))
	return ours
}

// hostsBlockPresent diz se o /etc/hosts tem domínios no bloco do HyPHP.
func hostsBlockPresent() bool {
	raw, err := os.ReadFile(netcfg.HostsPath)
	return err == nil && len(netcfg.ParseHostsBlock(string(raw))) > 0
}

// RemoveSystemChanges desfaz, com uma senha, a regra de DNS ou o bloco do
// hosts, o PATH, a confiança na CA e o limite das portas, e apaga a pasta
// cli, que é do usuário.
func (s *Stack) RemoveSystemChanges(ctx context.Context) error {
	if err := runUninstall(nil); err != nil {
		return err
	}
	return s.afterSystemRemoval(ctx)
}

// configuredPorts são as portas que o HyPHP abre, na ordem de Configurações.
func configuredPorts(st state.State) []int {
	return []int{st.HTTPPort, st.HTTPSPort, st.MySQLPort, st.MailpitSMTPPort, st.MailpitHTTPPort, st.PhpMyAdminPort}
}

// portsBelowLimit avisa das portas configuradas abaixo do limite que o Linux
// deixa abrir sem root (1024 na instalação padrão): o web server cairia ao
// subir. Ler o limite não pede senha; baixá-lo é ApplyPorts, por botão.
func portsBelowLimit(st state.State) []Warning {
	limit, err := netcfg.UnprivilegedPortStart()
	if err != nil {
		return nil
	}
	var below []string
	for _, p := range configuredPorts(st) {
		if p > 0 && p < limit {
			below = append(below, strconv.Itoa(p))
		}
	}
	if len(below) == 0 {
		return nil
	}
	return []Warning{{Code: "ports-pending", Message: i18n.T("warn.portsPending", limit, strings.Join(below, ", "))}}
}

// ApplyPorts baixa o limite das portas sem root até a menor porta
// configurada, pelo helper, com a senha do usuário. Vale para a máquina toda
// e sobrevive ao boot (/etc/sysctl.d). Depois sobe de novo o que caiu por
// causa da porta.
func (s *Stack) ApplyPorts(ctx context.Context) error {
	limit, err := netcfg.UnprivilegedPortStart()
	if err != nil {
		return fmt.Errorf("stack: ler o limite das portas: %w", err)
	}
	lowest := limit
	for _, p := range configuredPorts(s.State()) {
		if p > 0 && p < lowest {
			lowest = p
		}
	}
	if lowest == limit {
		return nil
	}
	helper, err := elevate.HelperPath()
	if err != nil {
		return i18n.Errorf("err.stack.helperUnavailable", err)
	}
	switch err := elevate.RunElevated(helper, []string{"ports-write", "--start", strconv.Itoa(lowest)}); {
	case errors.Is(err, elevate.ErrElevationDenied):
		return errors.New(i18n.T("err.stack.portsCancelled"))
	case err != nil:
		return fmt.Errorf("helper ports-write: %w", err)
	}
	s.d.Logger.Info("stack: portas baixas liberadas", "start", lowest)
	if _, err := s.Reconcile(ctx); err != nil {
		return err
	}
	s.lock()
	started := s.started
	s.unlock()
	if !started {
		return nil
	}
	return s.StartAll(ctx)
}
