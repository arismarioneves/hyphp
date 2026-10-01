package supervisor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// procHandle no macOS guarda o grupo do serviço. Setpgid deixa o pid do
// processo principal como pgid, e todo descendente herda o grupo: um sinal
// para -pgid alcança a árvore inteira, como o Job Object no Windows.
type procHandle struct{ pgid int }

// attachSelf não tem equivalente no macOS: não existe "mate meus filhos se eu
// morrer". A limpeza de órfãos depois de um crash do app chega na M1.
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
// cmd.Wait() do laço run()). kqueue com NOTE_EXIT avisa na saída mesmo que o
// processo ainda seja zumbi, que é o mesmo momento em que o Windows sinaliza o
// handle do processo.
func waitExit(pid int, timeout time.Duration) bool {
	kq, err := syscall.Kqueue()
	if err != nil {
		return pollExit(pid, timeout)
	}
	defer syscall.Close(kq)
	ev := syscall.Kevent_t{Ident: uint64(pid), Filter: syscall.EVFILT_PROC, Flags: syscall.EV_ADD | syscall.EV_ONESHOT, Fflags: syscall.NOTE_EXIT}
	ts := syscall.NsecToTimespec(timeout.Nanoseconds())
	out := make([]syscall.Kevent_t, 1)
	for {
		n, err := syscall.Kevent(kq, []syscall.Kevent_t{ev}, out, &ts)
		switch {
		case err == syscall.ESRCH:
			return true // já saiu antes do registro
		case err == syscall.EINTR:
			continue
		case err != nil:
			return pollExit(pid, timeout)
		}
		return n > 0
	}
}

// pollExit é a reserva se o kqueue falhar: Kill(pid, 0) dá ESRCH quando o
// processo não existe mais.
func pollExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}
