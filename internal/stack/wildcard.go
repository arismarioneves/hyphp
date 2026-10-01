package stack

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"

	"hyphp/internal/elevate"
	"hyphp/internal/i18n"
	"hyphp/internal/netcfg"
	"hyphp/internal/project"
)

// dnsSuffix é o único sufixo servido pelo resolvedor local. Um resolvedor por
// sufixo bastaria, mas todos os domínios do produto terminam em .test (spec
// §10.2) e uma regra NRPT por sufixo é o que o Windows entende.
const dnsSuffix = ".test"

// dnsAddr é onde o resolvedor escuta. 127.0.0.1 em vez de 0.0.0.0 de propósito:
// um resolvedor de desenvolvimento não deve responder para a rede. Também é o
// que permite conviver com quem já ocupa a 53 em outro endereço — o WARP da
// Cloudflare escuta em 127.0.2.2/3, e o svchost em 0.0.0.0.
const dnsAddr = "127.0.0.1:53"

// wildcardDomains devolve os sufixos dos projetos com wildcard ligado.
func wildcardDomains(projs []project.Project) []string {
	var out []string
	for _, p := range projs {
		if p.Wildcard {
			out = append(out, p.Domain)
		}
	}
	sort.Strings(out)
	return out
}

// syncWildcard sobe ou derruba o resolvedor local conforme haja projeto com
// wildcard, e reporta se a regra NRPT ainda falta.
//
// O resolvedor não precisa de privilégio: no Windows portas abaixo de 1024 não
// são reservadas. A regra NRPT precisa, e por isso segue a mesma política do
// hosts e da CA (C18.42, C18.45) — o Reconcile só avisa, e a escrita acontece
// em ApplyWildcardDNS, por ação explícita do usuário.
func (s *Stack) syncWildcard(projs []project.Project) []Warning {
	domains := wildcardDomains(projs)

	if len(domains) == 0 {
		if s.resolver != nil {
			if err := s.resolver.Stop(); err != nil {
				s.d.Logger.Warn("parar resolvedor DNS", "err", err)
			}
			s.resolver = nil
		}
		return nil
	}

	if s.resolver == nil {
		r := netcfg.NewResolver(dnsAddr, dnsSuffix, net.IPv4(127, 0, 0, 1))
		if err := r.Start(); err != nil {
			// Porta 53 tomada em 127.0.0.1 é o caso do §15: outro resolvedor
			// local (WARP, Acrylic, dnsmasq) já está ali. Degradar com aviso é
			// melhor do que insistir: os domínios sem wildcard seguem pelo
			// hosts e o resto do produto continua funcionando.
			return []Warning{{
				Code:    "wildcard-unavailable",
				Message: i18n.T("warn.wildcardUnavailable", dnsAddr, err, strings.Join(domains, ", ")),
			}}
		}
		s.resolver = r
		s.d.Logger.Info("stack: resolvedor DNS no ar", "addr", dnsAddr, "suffix", dnsSuffix)
	}

	if s.nrptDone || nrptRuleExists(dnsSuffix) {
		return nil
	}
	return []Warning{{
		Code:    "wildcard-pending",
		Message: i18n.T("warn.wildcardPending", strings.Join(domains, ", ")),
	}}
}

// ApplyWildcardDNS registra a regra NRPT pelo helper elevado. É a terceira e
// última porta do produto que dispara UAC, junto de ApplyHosts e InstallCA.
func (s *Stack) ApplyWildcardDNS(ctx context.Context) error {
	s.lock()
	ativo := s.resolver != nil
	s.unlock()
	if !ativo {
		return errors.New(i18n.T("err.stack.resolverDown"))
	}

	helper, err := elevate.HelperPath()
	if err != nil {
		return i18n.Errorf("err.stack.helperUnavailable", err)
	}
	switch err := elevate.RunElevated(helper, []string{
		"nrpt-add", "--namespace", dnsSuffix, "--server", "127.0.0.1",
	}); {
	case errors.Is(err, elevate.ErrElevationDenied):
		return errors.New(i18n.T("err.stack.nrptAddCancelled"))
	case err != nil:
		return fmt.Errorf("helper nrpt-add: %w", err)
	}

	s.lock()
	s.nrptDone = true
	s.unlock()
	s.d.Logger.Info("stack: regra NRPT registrada", "namespace", dnsSuffix)

	_, rerr := s.Reconcile(ctx)
	return rerr
}

// RemoveWildcardDNS desfaz a regra NRPT. Existe porque regra de DNS órfã afeta
// o sistema inteiro, não só o HyPHP: sem uma forma de remover pela UI, quem
// desinstalar o produto fica com o namespace .test apontando para um
// resolvedor que não existe mais.
func (s *Stack) RemoveWildcardDNS(ctx context.Context) error {
	helper, err := elevate.HelperPath()
	if err != nil {
		return i18n.Errorf("err.stack.helperUnavailable", err)
	}
	switch err := elevate.RunElevated(helper, []string{"nrpt-remove", "--namespace", dnsSuffix}); {
	case errors.Is(err, elevate.ErrElevationDenied):
		return errors.New(i18n.T("err.stack.nrptDelCancelled"))
	case err != nil:
		return fmt.Errorf("helper nrpt-remove: %w", err)
	}

	s.lock()
	s.nrptDone = false
	s.unlock()
	s.d.Logger.Info("stack: regra NRPT removida", "namespace", dnsSuffix)

	_, rerr := s.Reconcile(ctx)
	return rerr
}

// Close libera recursos de rede do Stack. Sem isto o resolvedor segue com a
// porta 53 presa até o processo morrer — e no encerramento limpo do app o
// processo ainda vive por alguns instantes, tempo suficiente para a próxima
// execução falhar no bind.
//
// Desiste com ctx.Err() se o ctx vencer esperando uma operação em curso: no
// encerramento, esperar um Reconcile longo aqui travaria o app aberto, e o
// fim do processo solta a porta de qualquer jeito.
func (s *Stack) Close(ctx context.Context) error {
	if err := s.lockCtx(ctx); err != nil {
		return err
	}
	r := s.resolver
	s.resolver = nil
	s.unlock()
	if r == nil {
		return nil
	}
	return r.Stop()
}
