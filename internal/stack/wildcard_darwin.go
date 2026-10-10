package stack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"

	"hyphp/internal/elevate"
	"hyphp/internal/i18n"
	"hyphp/internal/netcfg"
	"hyphp/internal/project"
)

// resolverPort é a porta do resolvedor no Mac. Sem root o macOS recusa a 53
// em 127.0.0.1 (spike de 2026-10-10: Permission denied), e a 15353 fica fora
// da faixa efêmera (49152–65535), onde o sistema sorteia portas de saída.
const resolverPort = 15353

// dnsAddr no Mac: 127.0.0.1, como no Windows, na porta alta. Variável para os
// testes usarem porta efêmera.
var dnsAddr = net.JoinHostPort("127.0.0.1", strconv.Itoa(resolverPort))

type ruleState int

const (
	ruleMissing ruleState = iota
	ruleOurs
	ruleForeign
)

// resolverRuleState lê o /etc/resolver/test sem senha. A regra nossa para
// outra porta conta como ausente: precisa ser regravada para a porta atual.
func resolverRuleState() ruleState {
	raw, err := os.ReadFile(netcfg.ResolverRulePath)
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return ruleMissing
	}
	switch ours, port := netcfg.ParseResolverRule(string(raw)); {
	case !ours:
		return ruleForeign
	case port != resolverPort:
		return ruleMissing
	}
	return ruleOurs
}

// syncWildcard no Mac: sem /etc/hosts, o resolvedor é o caminho de todo
// domínio .test e fica sempre no ar. A regra exige senha e, como no Windows,
// o Reconcile só avisa; a escrita é ApplyWildcardDNS, por ação do usuário.
func (s *Stack) syncWildcard([]project.Project) []Warning {
	if err := s.ensureResolver(true); err != nil {
		return []Warning{{
			Code:    "wildcard-unavailable",
			Message: i18n.T("warn.resolverUnavailable", dnsAddr, err),
		}}
	}
	switch resolverRuleState() {
	case ruleOurs:
		return nil
	case ruleForeign:
		return []Warning{{Code: "wildcard-pending", Message: i18n.T("warn.resolverForeign", netcfg.ResolverRulePath)}}
	}
	return []Warning{{Code: "wildcard-pending", Message: i18n.T("warn.resolverPending")}}
}

// ApplyWildcardDNS grava o /etc/resolver/test pelo helper, com a senha do
// usuário.
func (s *Stack) ApplyWildcardDNS(ctx context.Context) error {
	s.lock()
	ativo := s.resolver != nil
	s.unlock()
	if !ativo {
		return errors.New(i18n.T("err.stack.resolverDownMac", dnsAddr))
	}
	helper, err := elevate.HelperPath()
	if err != nil {
		return i18n.Errorf("err.stack.helperUnavailable", err)
	}
	switch err := elevate.RunElevated(helper, []string{"resolver-write", "--port", strconv.Itoa(resolverPort)}); {
	case errors.Is(err, elevate.ErrElevationDenied):
		return errors.New(i18n.T("err.stack.dnsAddCancelledMac"))
	case err != nil:
		return fmt.Errorf("helper resolver-write: %w", err)
	}
	s.d.Logger.Info("stack: regra de DNS registrada", "file", netcfg.ResolverRulePath)
	_, rerr := s.Reconcile(ctx)
	return rerr
}

// RemoveWildcardDNS não tem botão no Mac: a regra sai pelo "Remover do
// sistema" (RemoveSystemChanges), junto com o PATH e a CA.
func (s *Stack) RemoveWildcardDNS(context.Context) error {
	return i18n.Errorf("err.mac.unavailable")
}
