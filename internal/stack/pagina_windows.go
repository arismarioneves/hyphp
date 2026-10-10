package stack

import (
	"os"

	"hyphp/internal/netcfg"
)

// resolucao no Windows: o domínio abre pelo bloco do hosts aplicado ou pela
// regra NRPT do .test, que só vale com o resolvedor no ar. Sem nenhum dos
// dois, o que falta é o hosts: é o caminho de todo projeto, com ou sem
// wildcard.
func (s *Stack) resolucao() (hosts []string, dns bool, motivo string) {
	if raw, err := os.ReadFile(netcfg.HostsPath); err == nil {
		hosts = netcfg.ParseHostsBlock(string(raw))
	}
	return hosts, s.resolver != nil && (s.nrptDone || nrptRuleExists(dnsSuffix)), "hosts"
}
