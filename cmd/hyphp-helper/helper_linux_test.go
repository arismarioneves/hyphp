package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
	"hyphp/internal/update"
	"hyphp/internal/version"
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

// pacoteFalso é um .deb de mentira na pasta do usuário, com o manifesto
// assinado ao lado e a pasta do root do update numa temporária. O dpkg-deb
// falso responde com campos; o apt-get falso guarda o conteúdo da cópia que
// recebeu e devolve aptOut e aptErr.
type pacoteFalso struct {
	deb        string
	rel        update.Latest
	chave      ed25519.PrivateKey // assina o manifesto
	adultera   bool               // muda o manifesto depois de assinado
	campos     string
	aptOut     []byte
	aptErr     error
	chamadas   [][]string
	copiaNoApt string
}

// A versão do pacote é a deste helper: o install-update recusa versão mais
// velha que a instalada, e o teste segue valendo quando a versão sobe.
func novoPacoteFalso(t *testing.T) *pacoteFalso {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	conteudo := "!<arch>\nconteúdo do pacote\n"
	soma := sha256.Sum256([]byte(conteudo))
	v := version.Current
	p := &pacoteFalso{
		deb:    filepath.Join(t.TempDir(), "hyphp-"+v+"-linux-amd64.deb"),
		chave:  priv,
		campos: "Package: hyphp\nVersion: " + v + "\nArchitecture: " + runtime.GOARCH + "\n",
	}
	p.rel = update.Latest{Schema: update.SchemaVersion, Release: update.Release{
		Version: v, Date: "2026-10-22", Notes: []string{"nota"},
		WindowsAMD64: &update.Artifact{Path: "hyphp-" + v + "-windows-amd64-setup.exe", Size: 1, SHA256: strings.Repeat("0", 64)},
		LinuxAMD64:   &update.Artifact{Path: filepath.Base(p.deb), Size: int64(len(conteudo)), SHA256: hex.EncodeToString(soma[:])},
	}}
	escrever(t, p.deb, conteudo)
	cache := filepath.Join(t.TempDir(), "var", "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	salvaDir, salvaCmd, salvaChave := updateCacheDir, commandOutput, updateKey
	updateCacheDir, updateKey = filepath.Join(cache, "hyphp"), pub
	commandOutput = func(name string, args ...string) ([]byte, error) {
		p.chamadas = append(p.chamadas, append([]string{name}, args...))
		switch name {
		case "dpkg-deb":
			return []byte(p.campos), nil
		case "env":
			raw, err := os.ReadFile(args[len(args)-1])
			if err != nil {
				t.Errorf("cópia ausente na hora do apt-get: %v", err)
			}
			p.copiaNoApt = string(raw)
			return p.aptOut, p.aptErr
		}
		t.Errorf("comando inesperado: %s", name)
		return nil, errors.New("comando inesperado")
	}
	t.Setenv("PKEXEC_UID", strconv.Itoa(os.Getuid()))
	t.Cleanup(func() { updateCacheDir, commandOutput, updateKey = salvaDir, salvaCmd, salvaChave })
	return p
}

// args grava o manifesto e a assinatura como estão em p, ao lado do pacote, e
// devolve o pedido.
func (p *pacoteFalso) args(t *testing.T) []string {
	t.Helper()
	body, err := json.Marshal(p.rel)
	if err != nil {
		t.Fatal(err)
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(p.chave, body)) + "\n"
	if p.adultera {
		body = append(body, ' ')
	}
	manifesto := filepath.Join(t.TempDir(), "latest.json")
	escrever(t, manifesto, string(body))
	escrever(t, manifesto+".sig", sig)
	return []string{"install-update", "--deb", p.deb, "--manifesto", manifesto, "--assinatura", manifesto + ".sig"}
}

func TestInstallUpdateInstalaPeloAptGetACopiaConferida(t *testing.T) {
	p := novoPacoteFalso(t)
	if code, err := run(p.args(t)); code != exitOK {
		t.Fatalf("code = %d, err = %v", code, err)
	}
	var apt []string
	for _, c := range p.chamadas {
		if slices.Contains(c, "apt-get") {
			apt = c
		}
	}
	if apt == nil {
		t.Fatalf("o apt-get não rodou: %q", p.chamadas)
	}
	if copia := apt[len(apt)-1]; filepath.Dir(copia) != updateCacheDir {
		t.Fatalf("o apt-get recebeu %s, fora da pasta do root", copia)
	}
	// O mesmo pacote de novo (o e2e) precisa ser reinstalado, e sem terminal
	// uma pergunta do debconf travaria a instalação.
	if !slices.Contains(apt, "--reinstall") || !slices.Contains(apt, "DEBIAN_FRONTEND=noninteractive") {
		t.Fatalf("apt-get = %q", apt)
	}
	if p.copiaNoApt != ler(t, p.deb) {
		t.Fatal("a cópia instalada difere do pacote conferido")
	}
	if existe(updateCacheDir) {
		t.Fatal("a cópia ficou na pasta do root")
	}
}

// Nada chega ao dpkg-deb nem ao apt-get sem que o manifesto tenha a
// assinatura da chave das releases e o arquivo seja do usuário que pediu e
// bata com o manifesto.
func TestInstallUpdateRecusaPacoteQueNaoConfere(t *testing.T) {
	casos := []struct {
		nome string
		mexe func(t *testing.T, p *pacoteFalso)
	}{
		{"assinatura de outra chave", func(t *testing.T, p *pacoteFalso) {
			_, outra, err := ed25519.GenerateKey(nil)
			if err != nil {
				t.Fatal(err)
			}
			p.chave = outra
		}},
		{"manifesto mexido depois de assinado", func(_ *testing.T, p *pacoteFalso) { p.adultera = true }},
		{"sha256 diferente do manifesto", func(_ *testing.T, p *pacoteFalso) { p.rel.LinuxAMD64.SHA256 = strings.Repeat("0", 64) }},
		{"tamanho diferente do manifesto", func(_ *testing.T, p *pacoteFalso) { p.rel.LinuxAMD64.Size = 3 }},
		{"pacote acima do limite", func(_ *testing.T, p *pacoteFalso) { p.rel.LinuxAMD64.Size = maxDebSize + 1 }},
		{"manifesto sem pacote do Linux", func(_ *testing.T, p *pacoteFalso) { p.rel.LinuxAMD64 = nil }},
		{"versão mais velha que a instalada", func(_ *testing.T, p *pacoteFalso) { p.rel.Version = "0.1.0" }},
		{"caminho relativo", func(_ *testing.T, p *pacoteFalso) { p.deb = filepath.Base(p.deb) }},
		{"link", func(t *testing.T, p *pacoteFalso) {
			link := filepath.Join(t.TempDir(), "hyphp.deb")
			if err := os.Symlink(p.deb, link); err != nil {
				t.Fatal(err)
			}
			p.deb = link
		}},
		{"arquivo de outro usuário", func(t *testing.T, _ *pacoteFalso) {
			t.Setenv("PKEXEC_UID", strconv.Itoa(os.Getuid()+1))
		}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			p := novoPacoteFalso(t)
			c.mexe(t, p)
			if code, err := run(p.args(t)); code != exitUsage {
				t.Fatalf("code = %d, quer %d (err = %v)", code, exitUsage, err)
			}
			if len(p.chamadas) != 0 {
				t.Fatalf("comandos = %q", p.chamadas)
			}
			if existe(updateCacheDir) {
				t.Fatal("a cópia ficou na pasta do root")
			}
		})
	}
}

// Um control de outro pacote, de outra versão que a do manifesto ou de outra
// arquitetura para antes do apt-get.
func TestInstallUpdateRecusaControlDiferenteDoEsperado(t *testing.T) {
	v := version.Current
	casos := map[string]string{
		"outro pacote":      "Package: outro\nVersion: " + v + "\nArchitecture: " + runtime.GOARCH + "\n",
		"outra versão":      "Package: hyphp\nVersion: 0.0.1\nArchitecture: " + runtime.GOARCH + "\n",
		"outra arquitetura": "Package: hyphp\nVersion: " + v + "\nArchitecture: all\n",
		"campo ausente":     "Package: hyphp\nVersion: " + v + "\n",
	}
	for nome, campos := range casos {
		t.Run(nome, func(t *testing.T) {
			p := novoPacoteFalso(t)
			p.campos = campos
			if code, err := run(p.args(t)); code != exitUsage {
				t.Fatalf("code = %d, quer %d (err = %v)", code, exitUsage, err)
			}
			if len(p.chamadas) != 1 || p.chamadas[0][0] != "dpkg-deb" {
				t.Fatalf("comandos = %q, quer só o dpkg-deb", p.chamadas)
			}
			if existe(updateCacheDir) {
				t.Fatal("a cópia ficou na pasta do root")
			}
		})
	}
}

// O erro leva só o fim da saída do apt-get, onde fica o motivo.
func TestInstallUpdateFalhaDoAptGetDevolveOFimDaSaida(t *testing.T) {
	p := novoPacoteFalso(t)
	var saida strings.Builder
	for i := range 40 {
		fmt.Fprintf(&saida, "linha %02d\n", i+1)
	}
	p.aptOut, p.aptErr = []byte(saida.String()), errors.New("exit status 100")
	code, err := run(p.args(t))
	if code != exitIO {
		t.Fatalf("code = %d, quer %d", code, exitIO)
	}
	if msg := fmt.Sprint(err); !strings.Contains(msg, "linha 26") || !strings.Contains(msg, "linha 40") || strings.Contains(msg, "linha 25") {
		t.Fatalf("erro = %q, quer as linhas 26 a 40", msg)
	}
	if existe(updateCacheDir) {
		t.Fatal("a cópia ficou na pasta do root")
	}
}

// Um link no lugar da pasta do root levaria a cópia para onde o usuário
// alcança.
func TestInstallUpdateRecusaPastaDoRootQueELink(t *testing.T) {
	p := novoPacoteFalso(t)
	alvo := t.TempDir()
	if err := os.Symlink(alvo, updateCacheDir); err != nil {
		t.Fatal(err)
	}
	if code, err := run(p.args(t)); code != exitIO {
		t.Fatalf("code = %d, quer %d (err = %v)", code, exitIO, err)
	}
	if len(p.chamadas) != 0 {
		t.Fatalf("comandos = %q", p.chamadas)
	}
	if entradas, _ := os.ReadDir(alvo); len(entradas) != 0 {
		t.Fatalf("a cópia foi parar no alvo do link: %v", entradas)
	}
}
