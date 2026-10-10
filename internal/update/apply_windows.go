package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"hyphp/internal/elevate"
)

// updaterExe é a cópia do executável do app que aplica o update.
const updaterExe = "hyphp-updater.exe"

const (
	// createBreakawayFromJob tira o atualizador do Job Object kill-on-close do
	// app (spec §7.1); sem isso o kernel o mataria junto com o app, que é
	// exatamente o momento em que ele precisa estar vivo.
	createBreakawayFromJob = 0x01000000
	createNewProcessGroup  = 0x00000200

	installerTimeout = 10 * time.Minute
)

// Launch copia o executável em curso para updaterPath e o inicia em modo
// ApplyFlag, fora do Job Object. Quem chama só encerra o app depois que isto
// devolver nil: se falhar, o app continua aberto e o erro vai para a tela.
//
// A cópia é necessária porque o instalador sobrescreve o exe de req.Dir.
func Launch(req ApplyRequest, updaterPath string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("update: localizar o executável: %w", err)
	}
	if err := copyExe(self, updaterPath); err != nil {
		return fmt.Errorf("update: preparar o atualizador: %w", err)
	}
	cmd := exec.Command(updaterPath, append([]string{ApplyFlag}, req.args()...)...)
	cmd.Dir = filepath.Dir(updaterPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createBreakawayFromJob | createNewProcessGroup}
	if err := cmd.Start(); err != nil {
		// ERROR_ACCESS_DENIED aqui é o app rodando dentro de um job externo
		// que proíbe breakaway (algumas IDEs lançam assim).
		return fmt.Errorf("update: iniciar o atualizador fora do job do app: %w", err)
	}
	return cmd.Process.Release()
}

func apply(req ApplyRequest, logf func(string, ...any)) error {
	logf("aguardando o app (pid %d) sair", req.PID)
	if err := waitExit(req.PID, appExitTimeout); err != nil {
		return err
	}
	// var/update é gravável pelo usuário e a espera acima dura até 60 s: o
	// arquivo é reconferido logo antes de ser executado como administrador.
	if err := verifyFile(req.Installer, &Artifact{Size: req.Size, SHA256: req.SHA256}); err != nil {
		return err
	}
	// /D= precisa ser o último argumento e ir sem aspas, mesmo com espaços:
	// é regra do NSIS, e por isso a linha é montada à mão.
	params := "/S /D=" + req.Dir
	logf("rodando %s %s", req.Installer, params)
	code, err := elevate.RunInstaller(req.Installer, params, installerTimeout)
	switch {
	case errors.Is(err, elevate.ErrElevationDenied):
		return errors.New("atualização cancelada: a permissão do Windows (UAC) foi recusada")
	case err != nil:
		return fmt.Errorf("instalador: %w", err)
	case code != 0:
		return fmt.Errorf("o instalador terminou com código %d", code)
	}
	return nil
}

// relaunch abre de novo o exe de req.Dir: o novo, se o instalador rodou, ou
// o que ficou, se ele falhou.
func relaunch(req ApplyRequest) (string, error) {
	exe := filepath.Join(req.Dir, req.Exe)
	cmd := exec.Command(exe)
	cmd.Dir = req.Dir
	if err := cmd.Start(); err != nil {
		return exe, err
	}
	_ = cmd.Process.Release()
	return exe, nil
}

// waitExit espera o processo pid terminar. Processo que já não existe conta
// como terminado: o app pode ter saído antes de o atualizador chegar aqui.
func waitExit(pid int, timeout time.Duration) error {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)
	ev, err := windows.WaitForSingleObject(h, uint32(timeout/time.Millisecond))
	if err != nil {
		return fmt.Errorf("esperar o app sair: %w", err)
	}
	if ev != windows.WAIT_OBJECT_0 {
		return errAppAlive
	}
	return nil
}
