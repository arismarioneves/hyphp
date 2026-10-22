package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
	"hyphp/internal/update"
	"hyphp/internal/version"
)

const usage = "uso: hyphp-helper <resolved-write|hosts-write|profile-write|ca-trust|ports-write|install-update|uninstall> [flags]"

// portStartDefault é o limite das portas sem root que o kernel usa quando
// nenhum sysctl o muda.
const portStartDefault = 1024

// maxHostsSize limita o --from do hosts-write: o arquivo vem da pasta do
// usuário, e o helper o lê como root.
const maxHostsSize = 1 << 20

// maxDebSize limita o tamanho do pacote do install-update: o do HyPHP tem
// dezenas de MiB, e a cópia vai para uma pasta do root.
const maxDebSize = 512 << 20

// aptTailLines é quanto da saída do apt-get vai no erro: ela tem centenas de
// linhas, e o motivo da falha fica no fim.
const aptTailLines = 15

// updateCacheDir é a pasta do root onde o install-update copia o pacote antes
// de instalá-lo. Variável: os testes a apontam para uma pasta temporária.
var updateCacheDir = "/var/cache/hyphp"

// updateKey valida o manifesto do install-update. Variável: os testes
// assinam com uma chave própria.
var updateKey ed25519.PublicKey = update.PublicKey

// command roda um programa do sistema (systemctl, update-ca-certificates,
// sysctl). Variável: os testes guardam as chamadas sem mexer na máquina.
var command = func(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// commandOutput roda um programa e devolve a saída, stdout e stderr juntos:
// os campos do dpkg-deb e o relato do apt-get. Variável, como command.
var commandOutput = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
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
	case "install-update":
		return installUpdate(args[1:])
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
	raw, err := readOwnFile("--from", *from, maxHostsSize)
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

// readOwnFile lê um arquivo regular, de até limit bytes, do usuário que pediu
// a elevação; name é a flag que o trouxe, para as mensagens.
func readOwnFile(name, path string, limit int64) ([]byte, error) {
	f, _, err := openOwnFile(name, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("%s passa de %d bytes: %q", name, limit, path)
	}
	return raw, nil
}

// openOwnFile abre o arquivo regular path, do usuário que pediu a elevação;
// name é a flag que o trouxe, para as mensagens. O arquivo fica numa pasta do
// usuário, que pode trocá-lo a qualquer momento: a conferência e a leitura
// usam o mesmo descritor, aberto sem seguir link (que apontaria para um
// arquivo de outro) e sem travar num FIFO.
func openOwnFile(name, path string) (*os.File, os.FileInfo, error) {
	if !filepath.IsAbs(path) {
		return nil, nil, fmt.Errorf("%s precisa ser absoluto: %q", name, path)
	}
	uid, err := invokerUID()
	if err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", name, err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("%s: %w", name, err)
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	switch {
	case !st.Mode().IsRegular():
		f.Close()
		return nil, nil, fmt.Errorf("%s não é um arquivo comum: %q", name, path)
	case !ok || int(sys.Uid) != uid:
		f.Close()
		return nil, nil, fmt.Errorf("%s não é do usuário %d: %q", name, uid, path)
	}
	return f, st, nil
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

// installUpdate instala o pacote do update que o app baixou. O pedido vem de
// um processo do usuário, então nada nos argumentos é confiável: o sha256, o
// tamanho e a versão saem do manifesto, que precisa ter a assinatura da chave
// das releases. O pacote fica numa pasta do usuário: o helper o copia para
// updateCacheDir, confere a cópia (tamanho e sha256 do manifesto, nome, versão
// e arquitetura do control) e instala essa cópia, que o usuário não alcança.
func installUpdate(argv []string) (int, error) {
	fs := newFlags("install-update")
	deb := fs.String("deb", "", "pacote .deb baixado pelo app")
	manifest := fs.String("manifesto", "", "latest.json baixado pelo app")
	sig := fs.String("assinatura", "", "latest.json.sig baixado pelo app")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("install-update: %w", err)
	}
	rel, a, err := signedRelease(*manifest, *sig)
	if err != nil {
		return exitUsage, fmt.Errorf("install-update: %w", err)
	}
	f, st, err := openOwnFile("--deb", *deb)
	if err != nil {
		return exitUsage, fmt.Errorf("install-update: %w", err)
	}
	defer f.Close()
	if st.Size() != a.Size {
		return exitUsage, fmt.Errorf("install-update: o --deb tem %d bytes, e o manifesto diz %d", st.Size(), a.Size)
	}
	if err := ensureCacheDir(); err != nil {
		return exitIO, fmt.Errorf("install-update: %w", err)
	}
	// Só sai se estiver vazia: a pasta é do HyPHP, e o purge a apaga.
	defer os.Remove(updateCacheDir)
	tmp, err := os.CreateTemp(updateCacheDir, "hyphp-*.deb")
	if err != nil {
		return exitIO, fmt.Errorf("install-update: %w", err)
	}
	defer os.Remove(tmp.Name())
	if code, err := copyDeb(tmp, f, a.Size, a.SHA256); err != nil {
		return code, fmt.Errorf("install-update: %w", err)
	}
	if err := checkDebFields(tmp.Name(), rel.Version); err != nil {
		return exitUsage, fmt.Errorf("install-update: %w", err)
	}
	// O env põe o DEBIAN_FRONTEND só no apt-get: sem terminal, uma pergunta
	// do debconf travaria a instalação. O Lock::Timeout espera outro apt (a
	// atualização automática do sistema) em vez de falhar na hora.
	out, err := commandOutput("env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "--reinstall", "-o", "DPkg::Lock::Timeout=120", tmp.Name())
	if err != nil {
		return exitIO, fmt.Errorf("install-update: apt-get: %w\n%s", err, tailLines(out, aptTailLines))
	}
	return exitOK, nil
}

// signedRelease lê o manifesto e a assinatura da pasta do usuário, confere a
// assinatura com updateKey e devolve a versão e o pacote do Linux. Recusa
// versão mais velha que a deste helper, que é a instalada: um manifesto
// antigo, legítimo e assinado, não serve para voltar a uma versão com falhas
// já corrigidas.
func signedRelease(manifest, sig string) (update.Release, *update.Artifact, error) {
	body, err := readOwnFile("--manifesto", manifest, update.ManifestLimit)
	if err != nil {
		return update.Release{}, nil, err
	}
	raw, err := readOwnFile("--assinatura", sig, update.SigLimit)
	if err != nil {
		return update.Release{}, nil, err
	}
	if err := update.Verify(updateKey, body, raw); err != nil {
		return update.Release{}, nil, err
	}
	l, err := update.ParseLatest(body)
	if err != nil {
		return update.Release{}, nil, err
	}
	a := l.LinuxAMD64
	if a == nil {
		return update.Release{}, nil, errors.New("o manifesto não tem pacote linux_amd64")
	}
	if a.Size > maxDebSize {
		return update.Release{}, nil, fmt.Errorf("o pacote do manifesto tem %d bytes, acima do limite de %d", a.Size, maxDebSize)
	}
	c, err := update.Compare(l.Version, version.Current)
	if err != nil {
		return update.Release{}, nil, err
	}
	if c < 0 {
		return update.Release{}, nil, fmt.Errorf("o manifesto é da versão %s, mais velha que a %s instalada", l.Version, version.Current)
	}
	return l.Release, a, nil
}

// ensureCacheDir cria updateCacheDir com 0755, para o usuário _apt do apt-get
// ler a cópia. Uma pasta que já existe precisa ser do próprio helper (o root,
// pelo pkexec) e não um link: outra apontaria a cópia para onde o usuário
// alcança.
func ensureCacheDir() error {
	err := os.Mkdir(updateCacheDir, 0o755)
	if err == nil {
		// O umask herdado pode ter tirado a leitura dos outros.
		return os.Chmod(updateCacheDir, 0o755)
	}
	if !errors.Is(err, os.ErrExist) {
		return err
	}
	st, err := os.Lstat(updateCacheDir)
	if err != nil {
		return err
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !st.IsDir() || !ok || int(sys.Uid) != os.Geteuid() {
		return fmt.Errorf("%s existe e não é uma pasta do root", updateCacheDir)
	}
	return nil
}

// copyDeb copia src para dst conferindo tamanho e sha256 no caminho, e fecha
// dst. O limite size+1 revela um arquivo que cresceu depois do fstat.
func copyDeb(dst *os.File, src io.Reader, size int64, sum string) (int, error) {
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, h), io.LimitReader(src, size+1))
	if err == nil {
		// O _apt lê a cópia.
		err = dst.Chmod(0o644)
	}
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return exitIO, err
	}
	if n != size {
		return exitUsage, fmt.Errorf("o --deb tem %d bytes, e o manifesto diz %d", n, size)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != sum {
		return exitUsage, fmt.Errorf("o sha256 do --deb é %s, e o manifesto diz %s", got, sum)
	}
	return exitOK, nil
}

// checkDebFields confere o control do pacote: um .deb de outro programa, de
// outra arquitetura ou de outra versão que a anunciada (o app acharia de novo
// uma versão "nova": loop de update) não é instalado.
func checkDebFields(deb, versao string) error {
	out, err := commandOutput("dpkg-deb", "--field", deb, "Package", "Version", "Architecture")
	if err != nil {
		return fmt.Errorf("dpkg-deb: %w: %s", err, strings.TrimSpace(string(out)))
	}
	// Com mais de um campo pedido, o dpkg-deb imprime "Campo: valor".
	got := map[string]string{}
	for line := range strings.Lines(string(out)) {
		if k, v, ok := strings.Cut(line, ":"); ok {
			got[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	for _, c := range [...]struct{ field, want string }{
		{"Package", "hyphp"},
		{"Version", versao},
		{"Architecture", runtime.GOARCH},
	} {
		if got[c.field] != c.want {
			return fmt.Errorf("o pacote traz %s %q, e o esperado é %q", c.field, got[c.field], c.want)
		}
	}
	return nil
}

// tailLines devolve as últimas n linhas de out.
func tailLines(out []byte, n int) string {
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
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
