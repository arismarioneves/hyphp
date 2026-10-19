package stack

import (
	"context"
	"net"
	"strconv"
	"strings"

	"hyphp/internal/i18n"
	"hyphp/internal/project"
)

// resolverPort é a porta do resolvedor no Linux: abaixo de 1024 o bind exige
// root, e a 15353 fica fora da faixa efêmera, como no Mac.
const resolverPort = 15353

// dnsAddr no Linux: 127.0.0.1 na porta alta. Variável para os testes usarem
// porta efêmera.
var dnsAddr = net.JoinHostPort("127.0.0.1", strconv.Itoa(resolverPort))

// syncWildcard sobe o resolvedor quando algum projeto tem wildcard e avisa que
// os subdomínios ainda não resolvem: o HyPHP ainda não registra a regra de DNS
// do .test no Linux. Sem wildcard, o resolvedor fica parado.
func (s *Stack) syncWildcard(projs []project.Project) []Warning {
	domains := wildcardDomains(projs)
	if err := s.ensureResolver(len(domains) > 0); err != nil {
		return []Warning{{
			Code:    "wildcard-unavailable",
			Message: i18n.T("warn.wildcardUnavailable", dnsAddr, err, strings.Join(domains, ", ")),
		}}
	}
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
