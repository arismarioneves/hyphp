package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"hyphp/internal/i18n"
)

// updaterExe é a cópia do executável do app que aplica o update.
const updaterExe = "hyphp-updater"

// installCmd é a instalação pelo Terminal, a saída quando o app não pode se
// trocar sozinho.
const installCmd = "curl -fsSL https://github.com/" + GitHubRepo + "/releases/latest/download/install.sh | sh"

// bundleOf devolve o .app que contém a pasta do executável (Contents/MacOS).
// O update troca o bundle inteiro; um executável fora de um .app (go run,
// binário de teste) não tem o que trocar.
func bundleOf(exeDir string) (string, error) {
	contents := filepath.Dir(exeDir)
	bundle := filepath.Dir(contents)
	if filepath.Base(exeDir) != "MacOS" || filepath.Base(contents) != "Contents" || filepath.Ext(bundle) != ".app" {
		return "", fmt.Errorf("o HyPHP não está rodando de dentro de um .app (%s)", exeDir)
	}
	return bundle, nil
}

// Launch confere que a pasta do app aceita a troca, copia o executável em
// curso para updaterPath e o inicia em modo ApplyFlag numa sessão própria.
// Quem chama só encerra o app depois que isto devolver nil: se falhar, o app
// continua aberto e o erro vai para a tela.
func Launch(req ApplyRequest, updaterPath string) error {
	bundle, err := bundleOf(req.Dir)
	if err != nil {
		return err
	}
	// A troca são dois renames na pasta do app (em geral /Applications). Sem
	// gravação nela, o app fecharia para nada; o install.sh usa o sudo.
	if err := checkWritable(filepath.Dir(bundle)); err != nil {
		return i18n.Errorf("update.semEscrita", filepath.Dir(bundle), installCmd)
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("update: localizar o executável: %w", err)
	}
	// Arquivo novo, nunca o mesmo reescrito: o macOS guarda a assinatura de
	// código por vnode, e um Mach-O sobrescrito no mesmo inode morre com
	// SIGKILL ao executar. A assinatura ad-hoc vai dentro do Mach-O, então a
	// cópia fora do bundle roda.
	if err := os.Remove(updaterPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("update: preparar o atualizador: %w", err)
	}
	if err := copyExe(self, updaterPath); err != nil {
		return fmt.Errorf("update: preparar o atualizador: %w", err)
	}
	if err := os.Chmod(updaterPath, 0o755); err != nil {
		return fmt.Errorf("update: preparar o atualizador: %w", err)
	}
	cmd := exec.Command(updaterPath, append([]string{ApplyFlag}, req.args()...)...)
	cmd.Dir = filepath.Dir(updaterPath)
	// Sessão própria: quando o app foi aberto pelo LaunchAgent do início
	// automático, o launchd encerra o grupo de processos do job assim que o
	// app sai, que é quando o atualizador precisa estar vivo.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("update: iniciar o atualizador: %w", err)
	}
	return cmd.Process.Release()
}

// checkWritable cria e apaga um arquivo em dir.
func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".hyphp-escrita-")
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(f.Name())
}

func apply(req ApplyRequest, logf func(string, ...any)) error {
	bundle, err := bundleOf(req.Dir)
	if err != nil {
		return err
	}
	logf("aguardando o app (pid %d) sair", req.PID)
	if err := waitExit(req.PID, appExitTimeout); err != nil {
		return err
	}
	// var/update é gravável pelo usuário e a espera acima dura até 60 s: o
	// dmg é reconferido logo antes de virar o app instalado.
	if err := verifyFile(req.Installer, &Artifact{Size: req.Size, SHA256: req.SHA256}); err != nil {
		return err
	}
	mnt, err := os.MkdirTemp("", "hyphp-dmg-")
	if err != nil {
		return err
	}
	defer os.Remove(mnt)
	logf("montando %s em %s", req.Installer, mnt)
	if out, err := exec.Command("/usr/bin/hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", mnt, req.Installer).CombinedOutput(); err != nil {
		return fmt.Errorf("montar o dmg: %w: %s", err, strings.TrimSpace(string(out)))
	}
	// -force: com a troca feita ou desistida, nada mais lê do volume.
	defer func() {
		if out, err := exec.Command("/usr/bin/hdiutil", "detach", mnt, "-force").CombinedOutput(); err != nil {
			logf("desmontar o dmg: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}()
	src, err := appInDMG(mnt)
	if err != nil {
		return err
	}
	v, err := bundleVersion(src)
	if err != nil {
		return err
	}
	// Um dmg com outra versão que a anunciada faria o app achar de novo uma
	// versão "nova" no próximo Check: loop de update.
	if v != req.To {
		return fmt.Errorf("o dmg traz a versão %s, e o manifesto anunciou a %s", v, req.To)
	}
	logf("trocando %s pela versão %s", bundle, v)
	if err := replaceBundle(src, bundle, os.Rename); err != nil {
		return fmt.Errorf("%w; atualize pelo Terminal: %s", err, installCmd)
	}
	return nil
}

// appInDMG acha o único .app na raiz do volume montado.
func appInDMG(mnt string) (string, error) {
	apps, err := filepath.Glob(filepath.Join(mnt, "*.app"))
	if err != nil {
		return "", err
	}
	if len(apps) != 1 {
		return "", fmt.Errorf("o dmg tem %d .app na raiz, quer 1", len(apps))
	}
	return apps[0], nil
}

// bundleVersion lê o CFBundleShortVersionString do Info.plist do bundle. O
// plutil lê o plist em XML ou binário.
func bundleVersion(app string) (string, error) {
	plist := filepath.Join(app, "Contents", "Info.plist")
	out, err := exec.Command("/usr/bin/plutil", "-extract", "CFBundleShortVersionString", "raw", "-o", "-", plist).Output()
	if err != nil {
		return "", fmt.Errorf("ler a versão de %s: %w", plist, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// replaceBundle troca dst pelo bundle src: copia src ao lado de dst e faz dois
// renames na mesma pasta, sem nunca deixar a pasta sem app. Se o segundo
// rename falhar, o app atual volta para o lugar. rename é os.Rename fora dos
// testes.
func replaceBundle(src, dst string, rename func(oldpath, newpath string) error) error {
	parent, name := filepath.Dir(dst), filepath.Base(dst)
	novo := filepath.Join(parent, "."+name+".novo")
	antigo := filepath.Join(parent, "."+name+".antigo")
	// Restos de uma troca interrompida (queda de energia no meio).
	for _, p := range []string{novo, antigo} {
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("limpar %s: %w", p, err)
		}
	}
	// ditto preserva a assinatura de código, os links e os atributos do bundle.
	if out, err := exec.Command("/usr/bin/ditto", src, novo).CombinedOutput(); err != nil {
		_ = os.RemoveAll(novo)
		return fmt.Errorf("copiar o app novo: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if err := rename(dst, antigo); err != nil {
		_ = os.RemoveAll(novo)
		return fmt.Errorf("afastar o app atual: %w", err)
	}
	if err := rename(novo, dst); err != nil {
		if rerr := rename(antigo, dst); rerr != nil {
			return fmt.Errorf("instalar o app novo: %w; e o atual não voltou de %s: %v", err, antigo, rerr)
		}
		_ = os.RemoveAll(novo)
		return fmt.Errorf("instalar o app novo: %w", err)
	}
	// Uma sobra aqui não atrapalha: a próxima troca a apaga antes de começar.
	_ = os.RemoveAll(antigo)
	return nil
}

// relaunch reabre o bundle pelo Launch Services, como um clique no Finder: o
// novo, se a troca deu certo, ou o que ficou, se ela falhou.
func relaunch(req ApplyRequest) (string, error) {
	bundle, err := bundleOf(req.Dir)
	if err != nil {
		return req.Dir, err
	}
	return bundle, exec.Command("/usr/bin/open", bundle).Run()
}

// waitExit espera o processo pid terminar. Processo que já não existe conta
// como terminado. O app não é filho do atualizador (é o contrário), então não
// vira zumbi aqui: o launchd o recolhe assim que ele sai.
func waitExit(pid int, timeout time.Duration) error {
	// kill(0, …) iria para o grupo de processos inteiro.
	if pid <= 0 {
		return nil
	}
	limite := time.Now().Add(timeout)
	for {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if time.Now().After(limite) {
			return errAppAlive
		}
		time.Sleep(200 * time.Millisecond)
	}
}
