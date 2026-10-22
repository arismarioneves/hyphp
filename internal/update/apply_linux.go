package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"hyphp/internal/elevate"
	"hyphp/internal/i18n"
)

// updaterExe é o nome da cópia do atualizador nos outros sistemas; o Linux
// ignora o caminho que Apply monta com ele (ver Launch).
const updaterExe = "hyphp-updater"

// packageDir é onde o pacote .deb instala o app, o helper e a CLI. Só um app
// rodando daqui é trocado pelo apt-get.
const packageDir = "/usr/lib/hyphp"

// releasesURL é a página da última versão, para quem não instalou pelo pacote.
const releasesURL = "https://github.com/" + GitHubRepo + "/releases/latest"

// Launch inicia o app instalado em modo ApplyFlag numa sessão própria. Não há
// cópia do executável: o dpkg grava o arquivo novo num inode novo, e o
// processo em curso segue com o antigo; rodar de /usr/lib/hyphp também evita
// uma pasta pessoal montada com noexec. Quem chama só encerra o app depois que
// isto devolver nil.
func Launch(req ApplyRequest, _ string) error {
	if req.Dir != packageDir {
		return i18n.Errorf("update.semPacote", releasesURL)
	}
	cmd := exec.Command(filepath.Join(req.Dir, req.Exe), append([]string{ApplyFlag}, req.args()...)...)
	cmd.Dir = req.Dir
	// Sessão própria: o atualizador sobrevive ao fim do app e do grupo de
	// processos dele (o terminal que o abriu, por exemplo).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("update: iniciar o atualizador: %w", err)
	}
	return cmd.Process.Release()
}

func apply(req ApplyRequest, logf func(string, ...any)) error {
	logf("aguardando o app (pid %d) sair", req.PID)
	if err := waitExit(req.PID, appExitTimeout); err != nil {
		return err
	}
	// var/update é gravável pelo usuário e a espera acima dura até 60 s: o
	// pacote é reconferido logo antes de ir para o helper. O helper não confia
	// nestes argumentos: confere a assinatura do manifesto guardado ao lado do
	// pacote e tira dele o sha256, o tamanho e a versão.
	if err := verifyFile(req.Installer, &Artifact{Size: req.Size, SHA256: req.SHA256}); err != nil {
		return err
	}
	manifest := filepath.Join(filepath.Dir(req.Installer), manifestFile)
	helper := filepath.Join(req.Dir, "hyphp-helper")
	logf("instalando %s pelo %s", req.Installer, helper)
	// O prazo do RunElevated vale só para a janela de senha: o apt-get, já
	// como root, roda o tempo que precisar.
	err := elevate.RunElevated(helper, []string{
		"install-update",
		"--deb", req.Installer,
		"--manifesto", manifest,
		"--assinatura", manifest + ".sig",
	})
	if errors.Is(err, elevate.ErrElevationDenied) {
		return i18n.Errorf("update.cancelado")
	}
	if err != nil {
		return fmt.Errorf("update: instalar o pacote: %w", err)
	}
	logf("pacote %s instalado", req.To)
	return nil
}

// relaunch abre o app instalado numa sessão própria: o novo, se o apt-get
// instalou, ou o que ficou, se a instalação falhou. A saída é a do
// atualizador, para o que o app escrever ao abrir não se perder.
func relaunch(req ApplyRequest) (string, error) {
	app := filepath.Join(req.Dir, req.Exe)
	cmd := exec.Command(app)
	cmd.Dir = req.Dir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return app, err
	}
	return app, cmd.Process.Release()
}

// waitExit espera o processo pid terminar. Kill(pid, 0) não serve: o kernel
// devolve sucesso para zumbi, e o app sai como zumbi enquanto quem o abriu
// (o terminal, o gerenciador de sessão) não o recolhe. Processo que já não
// existe, ou zumbi, conta como terminado.
func waitExit(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return nil
	}
	limite := time.Now().Add(timeout)
	for {
		if exited(pid) {
			return nil
		}
		if time.Now().After(limite) {
			return errAppAlive
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// exited lê o estado (campo 3) de /proc/<pid>/stat. O campo 2 é o nome entre
// parênteses e pode ter espaço e ')': o estado vem depois do último ')'.
func exited(pid int) bool {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}
	s := string(raw)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return false
	}
	campos := strings.Fields(s[i+1:])
	return len(campos) > 0 && (campos[0] == "Z" || campos[0] == "X")
}
