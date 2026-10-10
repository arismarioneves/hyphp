package stack

import (
	"context"
	"net"

	"hyphp/internal/netcfg"
)

// dnsSuffix é o único sufixo servido pelo resolvedor local. Um resolvedor por
// sufixo bastaria, mas todos os domínios do produto terminam em .test (spec
// §10.2) e uma regra NRPT por sufixo é o que o Windows entende.
const dnsSuffix = ".test"

// ensureResolver sobe ou derruba o resolvedor local. want=false com ele no ar
// o para e solta a porta; want=true sem ele tenta subir em dnsAddr, que é por
// SO (wildcard_windows.go, wildcard_darwin.go).
func (s *Stack) ensureResolver(want bool) error {
	if !want {
		if s.resolver != nil {
			if err := s.resolver.Stop(); err != nil {
				s.d.Logger.Warn("parar resolvedor DNS", "err", err)
			}
			s.resolver = nil
		}
		return nil
	}
	if s.resolver != nil {
		return nil
	}
	r := netcfg.NewResolver(dnsAddr, dnsSuffix, net.IPv4(127, 0, 0, 1))
	if err := r.Start(); err != nil {
		return err
	}
	s.resolver = r
	s.d.Logger.Info("stack: resolvedor DNS no ar", "addr", dnsAddr, "suffix", dnsSuffix)
	return nil
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
