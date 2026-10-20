package main

import (
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
)

const usage = "uso: hyphp-helper <resolved-write|hosts-write|profile-write|ca-trust|ports-write|uninstall> [flags]"

// portStartDefault é o limite das portas sem root que o kernel usa quando
// nenhum sysctl o muda.
const portStartDefault = 1024

// maxHostsSize limita o --from do hosts-write: o arquivo vem da pasta do
// usuário, e o helper o lê como root.
const maxHostsSize = 1 << 20

// command roda um programa do sistema (systemctl, update-ca-certificates,
// sysctl). Variável: os testes guardam as chamadas sem mexer na máquina.
var command = func(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func main() {
	// Como no Mac, o stdout do helper chega ao app pelo pkexec: não há --result.
	code, err := run(os.Args[1:])
	report("", code, err)
	os.Exit(code)
}

func run(args []string) (int, error) {
	if len(args) == 0 {
		return exitUsage, errors.New(usage)
	}
	switch args[0] {
	case "resolved-write":
		return resolvedWrite(args[1:])
	case "hosts-write":
		return hostsWrite(args[1:])
	case "profile-write":
		return profileWrite(args[1:])
	case "ca-trust":
		return caTrust(args[1:])
	case "ports-write":
		return portsWrite(args[1:])
	case "uninstall":
		return uninstall(args[1:])
	default:
		return exitUsage, fmt.Errorf("subcomando desconhecido %q; %s", args[0], usage)
	}
}

// invokerUID é o usuário que pediu a elevação: o pkexec o põe em PKEXEC_UID,
// e o sudo (o e2e do CI), em SUDO_UID. Os arquivos que o helper lê e as pastas
// que ele põe no PATH precisam ser desse usuário.
func invokerUID() (int, error) {
	for _, key := range []string{"PKEXEC_UID", "SUDO_UID"} {
		if v := os.Getenv(key); v != "" {
			uid, err := strconv.Atoi(v)
			if err != nil {
				return 0, fmt.Errorf("%s inválido: %q", key, v)
			}
			return uid, nil
		}
	}
	return 0, errors.New("sem PKEXEC_UID nem SUDO_UID: o helper roda pelo pkexec")
}

// resolvedWrite grava o drop-in do systemd-resolved que manda o .test para o
// resolvedor do HyPHP, e reinicia o resolved para ele valer. Só a porta vem de
// fora: o conteúdo é montado aqui.
func resolvedWrite(argv []string) (int, error) {
	fs := newFlags("resolved-write")
	port := fs.Int("port", 0, "porta do resolvedor do HyPHP")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("resolved-write: %w", err)
	}
	if *port < 1024 || *port > 65535 {
		return exitUsage, fmt.Errorf("resolved-write: --port fora de 1024..65535: %d", *port)
	}
	if err := writeFileAtomic(netcfg.ResolvedDropInPath, []byte(netcfg.RenderResolvedDropIn(*port)), outsideDir(netcfg.ResolvedDropInPath)); err != nil {
		return exitIO, fmt.Errorf("resolved-write: %w", err)
	}
	if err := command("systemctl", "restart", "systemd-resolved"); err != nil {
		return exitIO, fmt.Errorf("resolved-write: %w", err)
	}
	return exitOK, nil
}

// hostsWrite troca o /etc/hosts pelo conteúdo que o app montou em --from, no
// Linux sem systemd-resolved. O arquivo tem de ser do usuário que pediu, e o
// conteúdo passa pela mesma validação do Windows.
func hostsWrite(argv []string) (int, error) {
	fs := newFlags("hosts-write")
	from := fs.String("from", "", "hosts montado pelo app")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("hosts-write: %w", err)
	}
	raw, err := readOwnFile(*from)
	if err != nil {
		return exitUsage, fmt.Errorf("hosts-write: %w", err)
	}
	if err := netcfg.ValidateHostsContent(string(raw)); err != nil {
		return exitUsage, fmt.Errorf("hosts-write: %w", err)
	}
	// O hosts é lido pelo nome, não por pasta: o temporário pode ficar ao lado.
	if err := writeFileAtomic(netcfg.HostsPath, raw, filepath.Dir(netcfg.HostsPath)); err != nil {
		return exitIO, fmt.Errorf("hosts-write: %w", err)
	}
	return exitOK, nil
}

// readOwnFile lê um arquivo regular, de até maxHostsSize, do usuário que
// pediu a elevação. O arquivo fica numa pasta do usuário, que pode trocá-lo a
// qualquer momento: a conferência e a leitura usam o mesmo descritor, aberto
// sem seguir link (que apontaria para um arquivo de outro) e sem travar num
// FIFO.
func readOwnFile(path string) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("--from precisa ser absoluto: %q", path)
	}
	uid, err := invokerUID()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("--from: %w", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("--from: %w", err)
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	switch {
	case !st.Mode().IsRegular():
		return nil, fmt.Errorf("--from não é um arquivo comum: %q", path)
	case !ok || int(sys.Uid) != uid:
		return nil, fmt.Errorf("--from não é do usuário %d: %q", uid, path)
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxHostsSize+1))
	if err != nil {
		return nil, fmt.Errorf("--from: %w", err)
	}
	if len(raw) > maxHostsSize {
		return nil, fmt.Errorf("--from passa de %d bytes: %q", maxHostsSize, path)
	}
	return raw, nil
}

// profileWrite grava o /etc/profile.d/hyphp.sh com a pasta cli do usuário.
func profileWrite(argv []string) (int, error) {
	fs := newFlags("profile-write")
	dir := fs.String("dir", "", "pasta cli do HyPHP")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("profile-write: %w", err)
	}
	owner, err := validCliDir(*dir)
	if err != nil {
		return exitUsage, fmt.Errorf("profile-write: %w", err)
	}
	if strings.ContainsAny(*dir, paths.ProfileDUnsafe) {
		return exitUsage, fmt.Errorf("profile-write: --dir com caractere que o shell interpretaria: %q", *dir)
	}
	uid, err := invokerUID()
	if err != nil {
		return exitUsage, fmt.Errorf("profile-write: %w", err)
	}
	if owner != uid {
		return exitUsage, fmt.Errorf("profile-write: --dir não é do usuário %d: %q", uid, *dir)
	}
	if err := writeFileAtomic(paths.ProfileDFile, []byte(paths.ProfileDContent(*dir, owner)), outsideDir(paths.ProfileDFile)); err != nil {
		return exitIO, fmt.Errorf("profile-write: %w", err)
	}
	return exitOK, nil
}

// caTrust copia a CA do mkcert para as âncoras do sistema e refaz o pacote de
// CAs, que curl, git e o resto do sistema leem. Grava o certificado validado,
// e não o arquivo relido.
func caTrust(argv []string) (int, error) {
	fs := newFlags("ca-trust")
	cert := fs.String("cert", "", "rootCA.pem do mkcert")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("ca-trust: %w", err)
	}
	ca, err := validCACert(*cert)
	if err != nil {
		return exitUsage, fmt.Errorf("ca-trust: %w", err)
	}
	anchor := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})
	if err := writeFileAtomic(netcfg.CAAnchorPath, anchor, outsideDir(netcfg.CAAnchorPath)); err != nil {
		return exitIO, fmt.Errorf("ca-trust: %w", err)
	}
	if err := command("update-ca-certificates"); err != nil {
		return exitIO, fmt.Errorf("ca-trust: %w", err)
	}
	return exitOK, nil
}

// portsWrite baixa o limite das portas sem root para --start, agora e a cada
// boot (/etc/sysctl.d). O limite vale para a máquina toda, não só para o
// HyPHP.
func portsWrite(argv []string) (int, error) {
	fs := newFlags("ports-write")
	start := fs.Int("start", 0, "menor porta que o usuário pode abrir")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("ports-write: %w", err)
	}
	if *start < 1 || *start > portStartDefault {
		return exitUsage, fmt.Errorf("ports-write: --start fora de 1..%d: %d", portStartDefault, *start)
	}
	if err := writeFileAtomic(netcfg.SysctlPortsPath, []byte(netcfg.RenderSysctlPorts(*start)), outsideDir(netcfg.SysctlPortsPath)); err != nil {
		return exitIO, fmt.Errorf("ports-write: %w", err)
	}
	if err := setPortStart(*start); err != nil {
		return exitIO, fmt.Errorf("ports-write: %w", err)
	}
	return exitOK, nil
}

// setPortStart muda o limite em uso, sem esperar o próximo boot.
func setPortStart(start int) error {
	return os.WriteFile(netcfg.UnprivilegedPortStartPath, []byte(strconv.Itoa(start)+"\n"), 0o644)
}

// uninstall desfaz o que o HyPHP gravou no sistema. Só sai o que tem a marca
// do HyPHP; o que já não existe conta como sucesso, e o botão pode ser usado
// depois de um desfazer parcial.
func uninstall(argv []string) (int, error) {
	fs := newFlags("uninstall")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("uninstall: %w", err)
	}
	steps := []func() error{removeResolvedDropIn, removeHostsBlock, removeProfileD, removeCAAnchor, removeSysctlPorts}
	for _, step := range steps {
		if err := step(); err != nil {
			return exitIO, fmt.Errorf("uninstall: %w", err)
		}
	}
	return exitOK, nil
}

// removeResolvedDropIn apaga o drop-in com a marca e reinicia o resolved.
func removeResolvedDropIn() error {
	raw, err := os.ReadFile(netcfg.ResolvedDropInPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if ours, _ := netcfg.ParseResolvedDropIn(string(raw)); !ours {
		return nil
	}
	if err := os.Remove(netcfg.ResolvedDropInPath); err != nil {
		return err
	}
	return command("systemctl", "restart", "systemd-resolved")
}

// removeHostsBlock tira do /etc/hosts o bloco do HyPHP; o resto fica byte a byte.
func removeHostsBlock() error {
	raw, err := os.ReadFile(netcfg.HostsPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(netcfg.ParseHostsBlock(string(raw))) == 0 {
		return nil
	}
	return writeFileAtomic(netcfg.HostsPath, []byte(netcfg.RenderHostsBlock(string(raw), nil)), filepath.Dir(netcfg.HostsPath))
}

// removeProfileD apaga o /etc/profile.d/hyphp.sh. O nome é do HyPHP: um
// arquivo com outro conteúdo, deixado por outro root, também sai.
func removeProfileD() error {
	if err := os.Remove(paths.ProfileDFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// removeCAAnchor apaga a âncora e refaz o pacote de CAs do zero: o --fresh
// tira do pacote o certificado que a âncora tinha posto.
func removeCAAnchor() error {
	err := os.Remove(netcfg.CAAnchorPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return command("update-ca-certificates", "--fresh")
}

// removeSysctlPorts apaga o arquivo do sysctl com a marca, volta o limite para
// o padrão do kernel e reaplica a configuração do sistema: uma ferramenta que
// grave outro limite no sysctl.d (o Podman sem root, por exemplo) volta ao
// dela.
func removeSysctlPorts() error {
	raw, err := os.ReadFile(netcfg.SysctlPortsPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if ours, _ := netcfg.ParseSysctlPorts(string(raw)); !ours {
		return nil
	}
	if err := os.Remove(netcfg.SysctlPortsPath); err != nil {
		return err
	}
	if err := setPortStart(portStartDefault); err != nil {
		return err
	}
	return command("sysctl", "--system")
}
