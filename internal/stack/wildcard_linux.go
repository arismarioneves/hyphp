package stack

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"hyphp/internal/elevate"
	"hyphp/internal/i18n"
	"hyphp/internal/netcfg"
	"hyphp/internal/project"
)

// resolverPort é a porta do resolvedor no Linux: abaixo de 1024 o bind exige
// root, e a 15353 fica fora da faixa efêmera, onde o sistema sorteia portas de
// saída.
const resolverPort = 15353

// dnsAddr no Linux: 127.0.0.1 na porta alta. Variável para os testes usarem
// porta efêmera.
var dnsAddr = net.JoinHostPort("127.0.0.1", strconv.Itoa(resolverPort))

// dropInActive diz se o drop-in do resolved com a marca do HyPHP aponta para
// a porta atual. Ler não pede senha.
func dropInActive() bool {
	raw, err := os.ReadFile(netcfg.ResolvedDropInPath)
	if err != nil {
		return false
	}
	ours, port := netcfg.ParseResolvedDropIn(string(raw))
	return ours && port == resolverPort
}

// syncWildcard no Linux. Com o systemd-resolved, o resolvedor é o caminho de
// todo domínio .test e fica sempre no ar; a regra exige senha e, como no Mac,
// o Reconcile só avisa. Sem o resolved não há regra a gravar: os domínios saem
// pelo /etc/hosts, que não aceita curinga, e o resolvedor fica parado.
func (s *Stack) syncWildcard(projs []project.Project) []Warning {
	if manageHosts() {
		if err := s.ensureResolver(false); err != nil {
			s.d.Logger.Warn("parar resolvedor DNS", "err", err)
		}
		domains := wildcardDomains(projs)
		if len(domains) == 0 {
			return nil
		}
		return []Warning{{
			Code:    "wildcard-unavailable",
			Message: i18n.T("warn.wildcardNoResolved", strings.Join(domains, ", ")),
		}}
	}
	if err := s.ensureResolver(true); err != nil {
		return []Warning{{
			Code:    "wildcard-unavailable",
			Message: i18n.T("warn.resolverUnavailable", dnsAddr, err),
		}}
	}
	if dropInActive() {
		return nil
	}
	return []Warning{{Code: "wildcard-pending", Message: i18n.T("warn.resolverPending")}}
}

// ApplyWildcardDNS grava o drop-in do resolved pelo helper, com a senha do
// usuário.
func (s *Stack) ApplyWildcardDNS(ctx context.Context) error {
	if manageHosts() {
		return errors.New(i18n.T("err.stack.dnsNoResolved"))
	}
	s.lock()
	ativo := s.resolver != nil
	s.unlock()
	if !ativo {
		return errors.New(i18n.T("err.stack.resolverDownLocal", dnsAddr))
	}
	helper, err := elevate.HelperPath()
	if err != nil {
		return i18n.Errorf("err.stack.helperUnavailable", err)
	}
	switch err := elevate.RunElevated(helper, []string{"resolved-write", "--port", strconv.Itoa(resolverPort)}); {
	case errors.Is(err, elevate.ErrElevationDenied):
		return errors.New(i18n.T("err.stack.dnsRuleCancelled"))
	case err != nil:
		return fmt.Errorf("helper resolved-write: %w", err)
	}
	s.d.Logger.Info("stack: regra de DNS registrada", "file", netcfg.ResolvedDropInPath)
	_, rerr := s.Reconcile(ctx)
	return rerr
}

// RemoveWildcardDNS não tem botão no Linux: a regra sai pelo "Remover do
// sistema" (RemoveSystemChanges), junto com o resto.
func (s *Stack) RemoveWildcardDNS(context.Context) error {
	return i18n.Errorf("err.stack.dnsRemoveBySystem")
}
