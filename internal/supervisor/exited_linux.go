package supervisor

// exited responde se o pid já encerrou. Kill(pid, 0) sozinho não serve: o
// kernel devolve sucesso para zumbi, e entre a saída e o cmd.Wait() do run() o
// processo é zumbi. Zumbi conta como encerrado porque quem colhe é o Wait do
// run(), como no Mac.
func exited(pid int) bool {
	if pid <= 0 {
		return true
	}
	st, err := readStat(pid)
	return err != nil || st.state == 'Z' || st.state == 'X'
}
