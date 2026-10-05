package supervisor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// procHandle no macOS guarda o grupo do serviço. Setpgid deixa o pid do
// processo principal como pgid, e todo descendente herda o grupo: um sinal
// para -pgid alcança a árvore inteira, como o Job Object no Windows.
type procHandle struct{ pgid int }

// attachSelf não tem equivalente no macOS: não existe "mate meus filhos se eu
// morrer". Os órfãos de um crash do app são encerrados na próxima abertura,
// pelo registro de processos (orphans_darwin.go).
func attachSelf() (procHandle, error) { return procHandle{}, nil }

func startProcess(spec Spec, out io.Writer) (*exec.Cmd, procHandle, error) {
	cmd := exec.Command(spec.Exe, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = append(os.Environ(), spec.Env...)
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, procHandle{}, fmt.Errorf("iniciar %s: %w", spec.ID, err)
	}
	return cmd, procHandle{pgid: cmd.Process.Pid}, nil
}

// stopProcess manda SIGTERM ao grupo (o mysqld e o php-fpm fecham com
// ordem), espera até timeout e, se o processo principal não saiu, manda
// SIGKILL ao grupo. Não chama cmd.Wait(): o laço run() é o dono dessa
// chamada, como no Windows.
func stopProcess(cmd *exec.Cmd, h procHandle, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = stopTimeout
	}
	if h.pgid <= 0 {
		return nil
	}
	if err := syscall.Kill(-h.pgid, syscall.SIGTERM); err == syscall.ESRCH {
		return nil
	}
	if waitExit(h.pgid, timeout) {
		return nil
	}
	_ = syscall.Kill(-h.pgid, syscall.SIGKILL)
	if !waitExit(h.pgid, time.Second) {
		return fmt.Errorf("processo %d não encerrou após SIGKILL", h.pgid)
	}
	return nil
}

// closeProcessHandle garante que nenhum descendente do grupo sobreviva ao
// serviço (um filho que ignorou SIGTERM e saiu do caminho do pai).
func closeProcessHandle(h procHandle) {
	if h.pgid > 0 {
		_ = syscall.Kill(-h.pgid, syscall.SIGKILL)
	}
}

// waitExit espera a saída do pid sem colher o status (quem colhe é o
// cmd.Wait() do laço run()). Consulta exited a cada 20 ms até o prazo. Um
// polling simples em vez de kqueue/NOTE_EXIT: no CI o kevent devolvia na hora
// (o registro voltava como evento EV_ERROR e era contado como saída), e o
// Stop retornava com o processo vivo.
func waitExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if exited(pid) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// sZomb é SZOMB de <sys/proc.h>: o processo saiu e espera o pai colher o
// status. x/sys/unix não exporta a constante.
const sZomb = 5

// exited responde se o pid já encerrou. Kill(pid, 0) sozinho não serve: o XNU
// devolve sucesso para zumbi (o POSIX manda), e entre a saída e o cmd.Wait()
// do run() o processo é zumbi. Zumbi conta como encerrado porque quem colhe é
// o Wait do run(); é o mesmo momento em que o Windows sinaliza o handle do
// processo (GetExitCodeProcess deixa de dar STILL_ACTIVE na saída, não na
// colheita).
func exited(pid int) bool {
	if pid <= 0 {
		return true
	}
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil { // EIO: o sysctl não achou o pid
		return true
	}
	return kp.Proc.P_pid != int32(pid) || kp.Proc.P_stat == sZomb
}
