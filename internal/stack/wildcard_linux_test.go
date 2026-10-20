package stack

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"hyphp/internal/i18n"
	"hyphp/internal/netcfg"
	"hyphp/internal/project"
	"hyphp/internal/state"
)

// stackLinux devolve um Stack só com logger, com o resolvedor numa porta
// efêmera e os arquivos de sistema numa pasta temporária. resolved diz se o
// resolv.conf aponta para o stub do systemd-resolved; dropIn vazio deixa o
// drop-in ausente.
func stackLinux(t *testing.T, resolved bool, dropIn string) *Stack {
	t.Helper()
	dir := t.TempDir()
	resolv := "nameserver 192.168.0.1\n"
	if resolved {
		resolv = "nameserver 127.0.0.53\n"
	}
	escreverArq(t, filepath.Join(dir, "resolv.conf"), resolv)
	if dropIn != "" {
		escreverArq(t, filepath.Join(dir, "hyphp.conf"), dropIn)
	}
	oldResolv, oldDropIn, oldAddr := netcfg.ResolvConfPath, netcfg.ResolvedDropInPath, dnsAddr
	netcfg.ResolvConfPath = filepath.Join(dir, "resolv.conf")
	netcfg.ResolvedDropInPath = filepath.Join(dir, "hyphp.conf")
	dnsAddr = "127.0.0.1:0"
	s := &Stack{d: Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	t.Cleanup(func() {
		_ = s.ensureResolver(false)
		netcfg.ResolvConfPath, netcfg.ResolvedDropInPath, dnsAddr = oldResolv, oldDropIn, oldAddr
	})
	return s
}

func escreverArq(t *testing.T, path, conteudo string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Com o resolved, todo .test passa pelo resolvedor: ele sobe mesmo sem projeto
// com wildcard, e a falta da regra vira o aviso com o botão.
func TestComResolvedSemRegraPedeARegraDeDNS(t *testing.T) {
	s := stackLinux(t, true, "")
	if manageHosts() {
		t.Fatal("com o resolved, o hosts não devia ser gerenciado")
	}
	warns := s.syncWildcard(nil)
	if s.resolver == nil {
		t.Fatal("resolvedor fora do ar com o resolved")
	}
	if len(warns) != 1 || warns[0].Code != "wildcard-pending" {
		t.Fatalf("warnings = %+v", warns)
	}
}

func TestComResolvedERegraDoHyPHPNaoAvisa(t *testing.T) {
	s := stackLinux(t, true, netcfg.RenderResolvedDropIn(resolverPort))
	if warns := s.syncWildcard(nil); len(warns) != 0 {
		t.Fatalf("warnings = %+v, quer nenhum", warns)
	}
	if _, dns, motivo := s.resolucao(); !dns || motivo != "dns" {
		t.Fatalf("resolucao: dns = %v, motivo = %q", dns, motivo)
	}
}

// A regra para outra porta manda as consultas para lugar nenhum: conta como
// ausente, e registrar de novo corrige.
func TestComResolvedRegraDeOutraPortaContaComoAusente(t *testing.T) {
	s := stackLinux(t, true, netcfg.RenderResolvedDropIn(5300))
	if warns := s.syncWildcard(nil); len(warns) != 1 || warns[0].Code != "wildcard-pending" {
		t.Fatalf("warnings = %+v", warns)
	}
}

// Sem o resolved, os domínios saem pelo hosts, que não aceita curinga: o
// aviso dos subdomínios não tem botão, o resolvedor fica parado e registrar a
// regra devolve o motivo em vez de pedir senha.
func TestSemResolvedWildcardFicaSemRegra(t *testing.T) {
	s := stackLinux(t, false, "")
	if !manageHosts() {
		t.Fatal("sem o resolved, o hosts devia ser gerenciado")
	}
	if warns := s.syncWildcard([]project.Project{{Manifest: project.Manifest{Domain: "acme.test"}}}); len(warns) != 0 {
		t.Fatalf("sem projeto com wildcard: warnings = %+v", warns)
	}
	warns := s.syncWildcard([]project.Project{{Manifest: project.Manifest{Domain: "acme.test", Wildcard: true}}})
	if len(warns) != 1 || warns[0].Code != "wildcard-unavailable" || warns[0].Message != i18n.T("warn.wildcardNoResolved", "acme.test") {
		t.Fatalf("warnings = %+v", warns)
	}
	if s.resolver != nil {
		t.Fatal("resolvedor no ar sem o resolved")
	}
	if err := s.ApplyWildcardDNS(t.Context()); err == nil || err.Error() != i18n.T("err.stack.dnsNoResolved") {
		t.Fatalf("ApplyWildcardDNS = %v", err)
	}
	if _, _, motivo := s.resolucao(); motivo != "hosts" {
		t.Fatalf("resolucao: motivo = %q, quer hosts", motivo)
	}
}

// Portas abaixo do limite derrubariam o serviço ao subir: o aviso cita só
// elas, e some quando o limite permite todas.
func TestPortasAbaixoDoLimiteAvisam(t *testing.T) {
	limite := filepath.Join(t.TempDir(), "ip_unprivileged_port_start")
	salvo := netcfg.UnprivilegedPortStartPath
	netcfg.UnprivilegedPortStartPath = limite
	t.Cleanup(func() { netcfg.UnprivilegedPortStartPath = salvo })

	st := state.State{HTTPPort: 80, HTTPSPort: 443, MySQLPort: 3306, MailpitSMTPPort: 1025, MailpitHTTPPort: 8025, PhpMyAdminPort: 8036}
	escreverArq(t, limite, "1024\n")
	warns := portsBelowLimit(st)
	if len(warns) != 1 || warns[0].Code != "ports-pending" || warns[0].Message != i18n.T("warn.portsPending", 1024, "80, 443") {
		t.Fatalf("warnings = %+v", warns)
	}
	escreverArq(t, limite, "80\n")
	if warns := portsBelowLimit(st); len(warns) != 0 {
		t.Fatalf("com o limite em 80: warnings = %+v", warns)
	}
	st.HTTPPort, st.HTTPSPort = 8080, 8443
	escreverArq(t, limite, "1024\n")
	if warns := portsBelowLimit(st); len(warns) != 0 {
		t.Fatalf("com portas altas: warnings = %+v", warns)
	}
}
