package stack

import (
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"

	"hyphp/internal/i18n"
	"hyphp/internal/netcfg"
)

// stackComRegra devolve um Stack só com logger, com o resolvedor numa porta
// efêmera e o /etc/resolver/test numa pasta temporária. conteudo vazio deixa
// o arquivo ausente.
func stackComRegra(t *testing.T, conteudo string) *Stack {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test")
	if conteudo != "" {
		if err := os.WriteFile(p, []byte(conteudo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oldPath, oldAddr := netcfg.ResolverRulePath, dnsAddr
	netcfg.ResolverRulePath, dnsAddr = p, "127.0.0.1:0"
	s := &Stack{d: Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	t.Cleanup(func() {
		_ = s.ensureResolver(false)
		netcfg.ResolverRulePath, dnsAddr = oldPath, oldAddr
	})
	return s
}

// No Mac o resolvedor é o caminho de todo domínio .test: sobe mesmo sem
// projeto com wildcard, e a falta da regra vira o aviso com o botão.
func TestResolvedorNoMacSemRegra(t *testing.T) {
	s := stackComRegra(t, "")
	warns := s.syncWildcard(nil)
	if s.resolver == nil {
		t.Fatal("resolvedor fora do ar sem projetos")
	}
	if len(warns) != 1 || warns[0].Code != "wildcard-pending" || warns[0].Message != i18n.T("warn.resolverPending") {
		t.Fatalf("warnings = %+v", warns)
	}
}

func TestResolvedorNoMacComARegraDoHyPHP(t *testing.T) {
	s := stackComRegra(t, netcfg.RenderResolverRule(resolverPort))
	if warns := s.syncWildcard(nil); len(warns) != 0 {
		t.Fatalf("warnings = %+v, quer nenhum", warns)
	}
}

// A regra para outra porta manda as consultas para lugar nenhum: conta como
// ausente, e registrar de novo corrige.
func TestResolvedorNoMacComRegraDeOutraPorta(t *testing.T) {
	s := stackComRegra(t, netcfg.RenderResolverRule(5300))
	if warns := s.syncWildcard(nil); len(warns) != 1 || warns[0].Message != i18n.T("warn.resolverPending") {
		t.Fatalf("warnings = %+v", warns)
	}
}

// Regra de outro programa: o aviso diz de quem é e que registrar substitui.
func TestResolvedorNoMacComRegraDeOutroPrograma(t *testing.T) {
	s := stackComRegra(t, "nameserver 127.0.0.1\n")
	want := i18n.T("warn.resolverForeign", netcfg.ResolverRulePath)
	if warns := s.syncWildcard(nil); len(warns) != 1 || warns[0].Code != "wildcard-pending" || warns[0].Message != want {
		t.Fatalf("warnings = %+v", warns)
	}
}

func TestResolvedorNoMacComPortaOcupada(t *testing.T) {
	s := stackComRegra(t, netcfg.RenderResolverRule(resolverPort))
	ocupada, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ocupada.Close()
	dnsAddr = ocupada.LocalAddr().String()
	if warns := s.syncWildcard(nil); len(warns) != 1 || warns[0].Code != "wildcard-unavailable" {
		t.Fatalf("warnings = %+v", warns)
	}
}

// No Mac os domínios resolvem pela regra de DNS: o /etc/hosts nunca é
// gravado, e a ação devolve o motivo em vez de pedir senha.
func TestAplicarHostsNoMac(t *testing.T) {
	s := &Stack{}
	if err := s.ApplyHosts(t.Context()); err == nil || err.Error() != i18n.T("err.stack.hostsMac") {
		t.Fatalf("ApplyHosts = %v", err)
	}
}
