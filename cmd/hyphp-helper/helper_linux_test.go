package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
)

// sistemaFalso aponta os arquivos de sistema para uma pasta temporária e
// guarda os comandos em vez de rodá-los: nenhum teste escreve em /etc, mexe
// no /proc ou reinicia serviço. Devolve os comandos rodados.
func sistemaFalso(t *testing.T) *[]string {
	t.Helper()
	dir := t.TempDir()
	salvos := []string{netcfg.ResolvedDropInPath, netcfg.HostsPath, paths.ProfileDFile, netcfg.CAAnchorPath, netcfg.SysctlPortsPath, netcfg.UnprivilegedPortStartPath}
	cmd := command
	netcfg.ResolvedDropInPath = filepath.Join(dir, "etc", "systemd", "resolved.conf.d", "hyphp.conf")
	netcfg.HostsPath = filepath.Join(dir, "etc", "hosts")
	paths.ProfileDFile = filepath.Join(dir, "etc", "profile.d", "hyphp.sh")
	netcfg.CAAnchorPath = filepath.Join(dir, "usr", "local", "share", "ca-certificates", "hyphp-mkcert.crt")
	netcfg.SysctlPortsPath = filepath.Join(dir, "etc", "sysctl.d", "60-hyphp.conf")
	netcfg.UnprivilegedPortStartPath = filepath.Join(dir, "proc", "ip_unprivileged_port_start")
	escrever(t, netcfg.UnprivilegedPortStartPath, "1024\n")
	rodados := &[]string{}
	command = func(name string, args ...string) error {
		*rodados = append(*rodados, strings.Join(append([]string{name}, args...), " "))
		return nil
	}
	t.Setenv("PKEXEC_UID", strconv.Itoa(os.Getuid()))
	t.Cleanup(func() {
		netcfg.ResolvedDropInPath, netcfg.HostsPath, paths.ProfileDFile = salvos[0], salvos[1], salvos[2]
		netcfg.CAAnchorPath, netcfg.SysctlPortsPath, netcfg.UnprivilegedPortStartPath = salvos[3], salvos[4], salvos[5]
		command = cmd
	})
	return rodados
}

func escrever(t *testing.T, path, conteudo string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}
}

func ler(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func existe(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestResolvedWriteGravaARegraEReiniciaOResolved(t *testing.T) {
	rodados := sistemaFalso(t)
	if code, err := run([]string{"resolved-write", "--port", "15353"}); code != exitOK {
		t.Fatalf("code = %d, err = %v", code, err)
	}
	ours, port := netcfg.ParseResolvedDropIn(ler(t, netcfg.ResolvedDropInPath))
	if !ours || port != 15353 {
		t.Fatalf("drop-in: nosso = %v, porta = %d", ours, port)
	}
	if strings.Join(*rodados, ";") != "systemctl restart systemd-resolved" {
		t.Fatalf("comandos = %q", *rodados)
	}
}

// O resolvedor do app roda sem root: abaixo de 1024 ele não escuta.
func TestResolvedWriteRecusaPortaPrivilegiada(t *testing.T) {
	sistemaFalso(t)
	if code, _ := run([]string{"resolved-write", "--port", "53"}); code != exitUsage {
		t.Fatalf("code = %d, quer %d", code, exitUsage)
	}
	if existe(netcfg.ResolvedDropInPath) {
		t.Fatal("drop-in gravado com porta recusada")
	}
}

func TestHostsWriteSoAceitaArquivoDoUsuarioQuePediu(t *testing.T) {
	sistemaFalso(t)
	escrever(t, netcfg.HostsPath, "127.0.0.1 localhost\n")
	novo := netcfg.RenderHostsBlock("127.0.0.1 localhost\n", []string{"acme.test"})
	from := filepath.Join(t.TempDir(), "hosts.next")
	escrever(t, from, novo)

	t.Setenv("PKEXEC_UID", strconv.Itoa(os.Getuid()+1))
	if code, _ := run([]string{"hosts-write", "--from", from}); code != exitUsage {
		t.Fatalf("arquivo de outro usuário: code = %d, quer %d", code, exitUsage)
	}
	link := filepath.Join(t.TempDir(), "hosts.next")
	if err := os.Symlink(from, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PKEXEC_UID", strconv.Itoa(os.Getuid()))
	if code, _ := run([]string{"hosts-write", "--from", link}); code != exitUsage {
		t.Fatalf("link: code = %d, quer %d", code, exitUsage)
	}
	if code, err := run([]string{"hosts-write", "--from", from}); code != exitOK {
		t.Fatalf("arquivo do usuário: code = %d, err = %v", code, err)
	}
	if got := ler(t, netcfg.HostsPath); got != novo {
		t.Fatalf("hosts = %q, quer %q", got, novo)
	}
}

func TestHostsWriteSemUsuarioQuePediuRecusa(t *testing.T) {
	sistemaFalso(t)
	t.Setenv("PKEXEC_UID", "")
	t.Setenv("SUDO_UID", "")
	from := filepath.Join(t.TempDir(), "hosts.next")
	escrever(t, from, "127.0.0.1 localhost\n")
	if code, _ := run([]string{"hosts-write", "--from", from}); code != exitUsage {
		t.Fatalf("code = %d, quer %d", code, exitUsage)
	}
}

// O profile.d é shell lido por todo shell de login: o caminho não pode trazer
// nada que o shell interpretaria, e a pasta tem de ser de quem pediu.
func TestProfileWriteAceitaSoAPastaCliDoUsuario(t *testing.T) {
	sistemaFalso(t)
	base := t.TempDir()
	cli := filepath.Join(base, "cli")
	perigosa := filepath.Join(t.TempDir(), "a$(reboot)", "cli")
	for _, d := range []string{cli, perigosa} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{"cli", filepath.Join(base, "bin"), perigosa} {
		if code, _ := run([]string{"profile-write", "--dir", dir}); code != exitUsage {
			t.Errorf("--dir %q: code = %d, quer %d", dir, code, exitUsage)
		}
	}
	t.Setenv("PKEXEC_UID", strconv.Itoa(os.Getuid()+1))
	if code, _ := run([]string{"profile-write", "--dir", cli}); code != exitUsage {
		t.Errorf("pasta de outro usuário: code = %d, quer %d", code, exitUsage)
	}
	if existe(paths.ProfileDFile) {
		t.Fatal("profile.d gravado com --dir recusado")
	}
	t.Setenv("PKEXEC_UID", strconv.Itoa(os.Getuid()))
	if code, err := run([]string{"profile-write", "--dir", cli}); code != exitOK {
		t.Fatalf("pasta cli válida: code = %d, err = %v", code, err)
	}
	if got, want := ler(t, paths.ProfileDFile), paths.ProfileDContent(cli, os.Getuid()); got != want {
		t.Fatalf("profile.d = %q, quer %q", got, want)
	}
}

// Só a CA validada vira âncora: um certificado a mais no fim do rootCA.pem
// entraria na confiança do sistema sem ninguém notar.
func TestCATrustPoeSoACANasAncorasERefazOPacote(t *testing.T) {
	rodados := sistemaFalso(t)
	cert := filepath.Join(t.TempDir(), "rootCA.pem")
	ca := string(certPEM(t, true))
	escrever(t, cert, ca+string(certPEM(t, true)))
	if code, err := run([]string{"ca-trust", "--cert", cert}); code != exitOK {
		t.Fatalf("code = %d, err = %v", code, err)
	}
	if got := ler(t, netcfg.CAAnchorPath); got != ca {
		t.Fatalf("âncora = %q, quer só a CA %q", got, ca)
	}
	if strings.Join(*rodados, ";") != "update-ca-certificates" {
		t.Fatalf("comandos = %q", *rodados)
	}
}

func TestPortsWriteBaixaOLimiteAgoraENoBoot(t *testing.T) {
	sistemaFalso(t)
	for _, start := range []string{"0", "1025", "x"} {
		if code, _ := run([]string{"ports-write", "--start", start}); code != exitUsage {
			t.Errorf("--start %s: code = %d, quer %d", start, code, exitUsage)
		}
	}
	if code, err := run([]string{"ports-write", "--start", "80"}); code != exitOK {
		t.Fatalf("code = %d, err = %v", code, err)
	}
	if ours, start := netcfg.ParseSysctlPorts(ler(t, netcfg.SysctlPortsPath)); !ours || start != 80 {
		t.Fatalf("sysctl.d: nosso = %v, limite = %d", ours, start)
	}
	if got, err := netcfg.UnprivilegedPortStart(); err != nil || got != 80 {
		t.Fatalf("limite em uso = %d (%v), quer 80", got, err)
	}
}

// Só sai o que tem a marca do HyPHP; o resto do hosts fica byte a byte, e o
// limite das portas volta ao padrão com a configuração do sistema reaplicada.
func TestUninstallDesfazSoOQueEDoHyPHP(t *testing.T) {
	rodados := sistemaFalso(t)
	hostsAlheio := "127.0.0.1 localhost\n# do usuário\n10.0.0.5 nas.lan\n"
	escrever(t, netcfg.HostsPath, netcfg.RenderHostsBlock(hostsAlheio, []string{"acme.test"}))
	escrever(t, netcfg.ResolvedDropInPath, netcfg.RenderResolvedDropIn(15353))
	escrever(t, paths.ProfileDFile, "# hyphp\n")
	escrever(t, netcfg.CAAnchorPath, "ca")
	escrever(t, netcfg.SysctlPortsPath, netcfg.RenderSysctlPorts(80))
	escrever(t, netcfg.UnprivilegedPortStartPath, "80\n")

	if code, err := run([]string{"uninstall"}); code != exitOK {
		t.Fatalf("code = %d, err = %v", code, err)
	}
	if got := ler(t, netcfg.HostsPath); got != hostsAlheio {
		t.Fatalf("hosts = %q, quer %q", got, hostsAlheio)
	}
	for _, p := range []string{netcfg.ResolvedDropInPath, paths.ProfileDFile, netcfg.CAAnchorPath, netcfg.SysctlPortsPath} {
		if existe(p) {
			t.Errorf("%s ficou", p)
		}
	}
	if got, _ := netcfg.UnprivilegedPortStart(); got != 1024 {
		t.Errorf("limite em uso = %d, quer 1024", got)
	}
	want := "systemctl restart systemd-resolved;update-ca-certificates --fresh;sysctl --system"
	if got := strings.Join(*rodados, ";"); got != want {
		t.Fatalf("comandos = %q, quer %q", got, want)
	}

	// De novo, sem nada a desfazer: sucesso, sem comando nenhum.
	*rodados = nil
	if code, err := run([]string{"uninstall"}); code != exitOK || len(*rodados) != 0 {
		t.Fatalf("segunda vez: code = %d, err = %v, comandos = %q", code, err, *rodados)
	}
}

// Um drop-in ou um sysctl.d sem a marca é de outro programa e fica.
func TestUninstallPreservaArquivosDeOutroPrograma(t *testing.T) {
	rodados := sistemaFalso(t)
	escrever(t, netcfg.ResolvedDropInPath, "[Resolve]\nDNS=1.1.1.1\n")
	escrever(t, netcfg.SysctlPortsPath, "net.ipv4.ip_unprivileged_port_start = 0\n")
	if code, err := run([]string{"uninstall"}); code != exitOK {
		t.Fatalf("code = %d, err = %v", code, err)
	}
	if !existe(netcfg.ResolvedDropInPath) || !existe(netcfg.SysctlPortsPath) {
		t.Fatal("apagou arquivo de outro programa")
	}
	if len(*rodados) != 0 {
		t.Fatalf("comandos = %q", *rodados)
	}
}
