package stack

import (
	"os"

	"hyphp/internal/netcfg"
)

// resolucao no Linux: com o systemd-resolved, todo .test depende do drop-in e
// do resolvedor no ar, como no Mac; sem ele, o domínio abre pelo bloco do
// /etc/hosts aplicado.
func (s *Stack) resolucao() (hosts []string, dns bool, motivo string) {
	if !manageHosts() {
		return nil, s.resolver != nil && dropInActive(), "dns"
	}
	if raw, err := os.ReadFile(netcfg.HostsPath); err == nil {
		hosts = netcfg.ParseHostsBlock(string(raw))
	}
	return hosts, false, "hosts"
}
