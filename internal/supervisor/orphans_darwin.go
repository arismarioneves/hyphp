package supervisor

import "golang.org/x/sys/unix"

// startTime devolve o horário de início do pid pelo kern.proc.pid.
func startTime(pid int) (sec, usec int64, err error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, 0, err
	}
	return kp.Proc.P_starttime.Sec, int64(kp.Proc.P_starttime.Usec), nil
}

// readProcInfo lê do kernel o que o shouldReap compara. Zumbi conta como
// morto, como em exited. O executável vem do kern.procargs2, que guarda o
// caminho passado ao execve.
func readProcInfo(pid int) procInfo {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kp.Proc.P_pid != int32(pid) || kp.Proc.P_stat == sZomb {
		return procInfo{}
	}
	info := procInfo{
		alive:     true,
		startSec:  kp.Proc.P_starttime.Sec,
		startUsec: int64(kp.Proc.P_starttime.Usec),
		pgid:      int(kp.Eproc.Pgid),
		ppid:      int(kp.Eproc.Ppid),
	}
	if buf, err := unix.SysctlRaw("kern.procargs2", pid); err == nil {
		info.exe = parseProcargs(buf)
	}
	return info
}
