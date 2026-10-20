package netcfg

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func arquivo(t *testing.T, conteudo string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// O drop-in só tem efeito quando a glibc consulta o stub do resolved primeiro;
// com o resolv.conf apontando antes para os servidores da rede, o .test sai
// pelo hosts.
func TestResolvedStubSoComOStubComoPrimeiroNameserver(t *testing.T) {
	casos := map[string]bool{
		"# gerado pelo resolved\nnameserver 127.0.0.53\noptions edns0 trust-ad\nsearch .\n": true,
		"nameserver 127.0.0.53\nnameserver 192.168.0.1\n":                                   true,
		"nameserver 192.168.0.1\nnameserver 127.0.0.53\n":                                   false,
		"nameserver 192.168.0.1\n":                                                          false,
		"# nameserver 127.0.0.53\nnameserver 1.1.1.1\n":                                     false,
		"": false,
	}
	salvo := ResolvConfPath
	t.Cleanup(func() { ResolvConfPath = salvo })
	for conteudo, quer := range casos {
		ResolvConfPath = arquivo(t, conteudo)
		if got := ResolvedStub(); got != quer {
			t.Errorf("%q: ResolvedStub = %v, quer %v", conteudo, got, quer)
		}
	}
	ResolvConfPath = filepath.Join(t.TempDir(), "nao-existe")
	if ResolvedStub() {
		t.Error("sem resolv.conf, ResolvedStub = true")
	}
}

func TestDropInDeOutroProgramaNaoEDoHyPHP(t *testing.T) {
	if ours, port := ParseResolvedDropIn(RenderResolvedDropIn(15353)); !ours || port != 15353 {
		t.Fatalf("o nosso: ours = %v, port = %d", ours, port)
	}
	if ours, _ := ParseResolvedDropIn("[Resolve]\nDNS=127.0.0.1:15353\nDomains=~test\n"); ours {
		t.Fatal("drop-in sem a marca contou como do HyPHP")
	}
}

func TestSysctlPortsLeOLimiteComOuSemEspacos(t *testing.T) {
	if ours, start := ParseSysctlPorts("# hyphp\nnet.ipv4.ip_unprivileged_port_start=80\n"); !ours || start != 80 {
		t.Fatalf("sem espaços: ours = %v, start = %d", ours, start)
	}
	if ours, start := ParseSysctlPorts("# podman\nnet.ipv4.ip_unprivileged_port_start = 0\n"); ours || start != 0 {
		t.Fatalf("de outro programa: ours = %v, start = %d", ours, start)
	}
	salvo := UnprivilegedPortStartPath
	t.Cleanup(func() { UnprivilegedPortStartPath = salvo })
	UnprivilegedPortStartPath = arquivo(t, "1024\n")
	if n, err := UnprivilegedPortStart(); err != nil || n != 1024 {
		t.Fatalf("UnprivilegedPortStart = %d, %v", n, err)
	}
}

// A âncora tem de ser a CA deste mkcert: uma CA recriada (outra chave) deixa
// a do sistema velha, e os sites sairiam com certificado não confiável.
func TestCAInstaladaSoComAAncoraDaMesmaCA(t *testing.T) {
	caroot := t.TempDir()
	m := Mkcert{CARoot: caroot}
	salvo := CAAnchorPath
	t.Cleanup(func() { CAAnchorPath = salvo })
	CAAnchorPath = filepath.Join(t.TempDir(), "hyphp-mkcert.crt")

	propria := selfSignedPEM(t, time.Now().Add(time.Hour))
	if err := os.WriteFile(filepath.Join(caroot, "rootCA.pem"), propria, 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.CAInstalled(); err != nil || ok {
		t.Fatalf("sem âncora: ok = %v, err = %v", ok, err)
	}
	if err := os.WriteFile(CAAnchorPath, selfSignedPEM(t, time.Now().Add(time.Hour)), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.CAInstalled(); err != nil || ok {
		t.Fatalf("âncora de outra CA: ok = %v, err = %v", ok, err)
	}
	if err := os.WriteFile(CAAnchorPath, propria, 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.CAInstalled(); err != nil || !ok {
		t.Fatalf("âncora da mesma CA: ok = %v, err = %v", ok, err)
	}
}
