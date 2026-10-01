package supervisor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// procHandle no Windows guarda o Job Object do serviço: é ele que alcança a
// árvore inteira, inclusive os netos que o processo principal criar.
type procHandle struct{ job windows.Handle }

// attachSelf põe o próprio hyphp.exe num job kill-on-close: se o app morrer
// de qualquer jeito, o kernel mata os serviços junto.
func attachSelf() (procHandle, error) {
	job, err := AttachSelfToKillOnCloseJob()
	return procHandle{job: job}, err
}

// closeProcessHandle fecha o job do serviço; KILL_ON_JOB_CLOSE mata o que
// sobrar da árvore.
func closeProcessHandle(h procHandle) {
	if h.job != 0 {
		_ = windows.CloseHandle(h.job)
	}
}

// startProcess cria o Job Object do serviço, inicia o processo com a janela
// oculta e o associa ao job. Devolve o cmd (cujo Wait é do chamador) e o
// handle do job (que o chamador fecha, matando o que sobrar da árvore).
//
// Por que NÃO usamos CREATE_SUSPENDED: o padrão Win32 de "criar suspenso,
// associar ao job, retomar a thread" exige ResumeThread no handle da thread
// principal, e o Go não expõe esse handle (os/exec guarda só o handle do
// processo). O filho já nasce dentro do job GLOBAL por herança
// (AttachSelfToKillOnCloseJob associou o próprio hyphp.exe), então a janela
// entre Start() e AssignProcessToJobObject afeta apenas o job POR SERVIÇO —
// um Stop disparado nesses microssegundos ainda encontra o processo, e um
// crash do hyphp.exe nesse instante é coberto pelo job global. Zero órfãos
// continua garantido pelo kernel.
func startProcess(spec Spec, out io.Writer) (*exec.Cmd, procHandle, error) {
	job, err := newKillOnCloseJob()
	if err != nil {
		return nil, procHandle{}, fmt.Errorf("job de %s: %w", spec.ID, err)
	}
	cmd := exec.Command(spec.Exe, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = append(os.Environ(), spec.Env...)
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
		// CREATE_NO_WINDOW é o que de fato impede o console: HideWindow só passa
		// SW_HIDE pelo STARTUPINFO, e o console de um processo console nasce
		// antes disso — cada serviço piscava uma janela ao subir, e são sete.
		// CREATE_NEW_PROCESS_GROUP continua necessário para o Ctrl-Break do Stop.
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW,
	}
	if err := cmd.Start(); err != nil {
		windows.CloseHandle(job)
		return nil, procHandle{}, fmt.Errorf("iniciar %s: %w", spec.Exe, err)
	}
	if err := assignPID(job, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		windows.CloseHandle(job)
		return nil, procHandle{}, fmt.Errorf("associar %s ao job: %w", spec.ID, err)
	}
	return cmd, procHandle{job: job}, nil
}

// stopProcess mata a árvore inteira do serviço e espera o processo principal
// morrer de fato, no máximo timeout.
//
// Por que não chamamos cmd.Wait() aqui: o laço run() é o único dono de
// cmd.Wait(); uma segunda chamada devolve "exec: Wait was already called"
// NA HORA, e o timeout perderia o sentido. Esperamos no próprio handle do
// processo, aberto ANTES de terminar o job (depois da morte o PID pode ser
// reciclado). Isso também deixa o Stop independente da drenagem de
// stdout/stderr: cmd.Wait() só retorna quando as goroutines de cópia dos
// pipes terminam, o que depende de todo descendente ter fechado sua ponta —
// garantido aqui porque TerminateJobObject mata a árvore, mas não é algo de
// que o Stop precise depender.
func stopProcess(cmd *exec.Cmd, proc procHandle, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = stopTimeout
	}
	var h windows.Handle
	if cmd != nil && cmd.Process != nil {
		opened, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
		if err == nil {
			h = opened
			defer windows.CloseHandle(h)
		}
	}
	if proc.job != 0 {
		if err := windows.TerminateJobObject(proc.job, 1); err != nil {
			return fmt.Errorf("TerminateJobObject: %w", err)
		}
	}
	if h == 0 {
		return nil
	}
	ev, err := windows.WaitForSingleObject(h, uint32(timeout/time.Millisecond))
	if err != nil {
		return fmt.Errorf("WaitForSingleObject: %w", err)
	}
	if ev == uint32(windows.WAIT_TIMEOUT) {
		return fmt.Errorf("processo %d não encerrou em %s", cmd.Process.Pid, timeout)
	}
	return nil
}
