package supervisor

import "golang.org/x/sys/unix"

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
