package stack

import (
	"os"

	"hyphp/internal/netcfg"
)

// resolucao no Linux: o domínio abre pelo bloco do /etc/hosts aplicado. A
// regra de DNS do .test ainda não é gerenciada, então nenhum domínio conta
// como resolvido pelo DNS.
func (s *Stack) resolucao() (hosts []string, dns bool, motivo string) {
	if raw, err := os.ReadFile(netcfg.HostsPath); err == nil {
		hosts = netcfg.ParseHostsBlock(string(raw))
	}
	return hosts, false, "hosts"
}
