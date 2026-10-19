package stack

import (
	"context"
	"strings"

	"hyphp/internal/i18n"
	"hyphp/internal/project"
)

// dnsAddr no Linux: 127.0.0.1 numa porta alta, fora da faixa efêmera, porque
// abaixo de 1024 o bind exige root.
const dnsAddr = "127.0.0.1:15353"

// syncWildcard avisa que os subdomínios dos projetos com wildcard ainda não
// resolvem: falta a regra de DNS do .test, que o HyPHP ainda não registra no
// Linux. Sem a regra ninguém consulta o resolvedor, então ele fica parado.
func (s *Stack) syncWildcard(projs []project.Project) []Warning {
	if err := s.ensureResolver(false); err != nil {
		s.d.Logger.Warn("parar resolvedor DNS", "err", err)
	}
	domains := wildcardDomains(projs)
	if len(domains) == 0 {
		return nil
	}
	return []Warning{{
		Code:    "wildcard-pending",
		Message: i18n.T("warn.wildcardPending", strings.Join(domains, ", ")),
	}}
}

// ApplyWildcardDNS no Linux ainda não existe.
func (s *Stack) ApplyWildcardDNS(context.Context) error {
	return i18n.Errorf("err.linux.unavailable")
}

// RemoveWildcardDNS no Linux ainda não existe.
func (s *Stack) RemoveWildcardDNS(context.Context) error {
	return i18n.Errorf("err.linux.unavailable")
}
