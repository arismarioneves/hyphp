package supervisor

import (
	"time"

	"golang.org/x/sys/unix"
)

// longRunningSpec é o par do "ping -n 30" do Windows: um sh com filhos
// (cada sleep), para o Stop ter uma árvore a matar, e que sai sozinho em
// ~30 s. Sem o limite, um teste que morresse no meio deixaria o laço vivo
// para sempre — no macOS não há job kill-on-close que o leve junto.
func longRunningSpec(id string) Spec {
	return Spec{
		ID:    id,
		Name:  id,
		Group: "teste",
		Exe:   "/bin/sh",
		Args:  []string{"-c", "i=0; while [ $i -lt 30 ]; do sleep 1; i=$((i+1)); done"},
		Probe: AliveProbe{Grace: 300 * time.Millisecond},
	}
}

// failingSpec sai com código 1 logo ao subir; o AliveProbe nunca chega a
// passar. Código 1 como no Windows: os testes conferem o código no LastError.
func failingSpec(id string) Spec {
	return Spec{
		ID:    id,
		Exe:   "/bin/sh",
		Args:  []string{"-c", "exit 1"},
		Probe: AliveProbe{Grace: 2 * time.Second},
	}
}

// echoSpec escreve text numa linha e sai.
func echoSpec(id, text string) Spec {
	return Spec{
		ID:    id,
		Exe:   "/bin/sh",
		Args:  []string{"-c", "echo " + text},
		Probe: AliveProbe{Grace: 2 * time.Second},
	}
}

// sZomb é SZOMB de <sys/proc.h>: o processo saiu e espera o pai colher o
// status. x/sys/unix não exporta a constante.
const sZomb = 5

// processAlive responde se o PID ainda está em execução. Kill(pid, 0) sozinho
// não serve: o XNU devolve sucesso para zumbi (o POSIX manda), e entre a
// saída e o cmd.Wait() o processo é zumbi — o teste veria vivo um processo já
// morto. Zumbi conta como morto, igual ao Windows, onde GetExitCodeProcess
// deixa de dar STILL_ACTIVE na saída e não na colheita.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil { // EIO: o sysctl não achou o pid
		return false
	}
	return kp.Proc.P_pid == int32(pid) && kp.Proc.P_stat != sZomb
}
