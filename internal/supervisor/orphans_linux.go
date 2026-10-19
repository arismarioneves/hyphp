package supervisor

import (
	"os"
	"strconv"
)

// startTime no Linux é o campo 22 de /proc/<pid>/stat, em ticks desde o boot:
// vai inteiro em sec, e usec fica zero. Identifica o processo dentro do mesmo
// boot, que é o que o ReapOrphans compara.
func startTime(pid int) (sec, usec int64, err error) {
	st, err := readStat(pid)
	if err != nil {
		return 0, 0, err
	}
	return st.startTicks, 0, nil
}

// readProcInfo lê do /proc o que o shouldReap compara. Zumbi conta como
// morto, como em exited. O /proc/<pid>/exe já vem com os symlinks resolvidos
// (opt/ do Homebrew vira o Cellar); o sameExecutable resolve o registrado.
func readProcInfo(pid int) procInfo {
	st, err := readStat(pid)
	if err != nil || st.state == 'Z' || st.state == 'X' {
		return procInfo{}
	}
	info := procInfo{
		alive:    true,
		startSec: st.startTicks,
		pgid:     st.pgid,
		ppid:     st.ppid,
	}
	if exe, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe"); err == nil {
		info.exe = exe
	}
	return info
}
