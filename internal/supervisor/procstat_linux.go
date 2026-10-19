package supervisor

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// procStat são os campos de /proc/<pid>/stat que o supervisor usa: estado (3),
// pai (4), grupo (5) e o início em ticks desde o boot (22).
type procStat struct {
	state      byte
	ppid, pgid int
	startTicks int64
}

func readStat(pid int) (procStat, error) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return procStat{}, err
	}
	return parseStat(string(raw))
}

// parseStat começa depois do último ')': o campo 2 é o nome do processo entre
// parênteses e pode ter espaço e ')' (o php-fpm troca o título para
// "php-fpm: master process (...)").
func parseStat(raw string) (procStat, error) {
	i := strings.LastIndexByte(raw, ')')
	if i < 0 {
		return procStat{}, errors.New("stat sem o nome do processo")
	}
	// rest[0] é o campo 3; o campo n fica em rest[n-3].
	rest := strings.Fields(raw[i+1:])
	if len(rest) < 20 || len(rest[0]) != 1 {
		return procStat{}, fmt.Errorf("stat curto: %d campos depois do nome", len(rest))
	}
	ppid, err1 := strconv.Atoi(rest[1])
	pgid, err2 := strconv.Atoi(rest[2])
	start, err3 := strconv.ParseInt(rest[19], 10, 64)
	if err := errors.Join(err1, err2, err3); err != nil {
		return procStat{}, fmt.Errorf("stat ilegível: %w", err)
	}
	return procStat{state: rest[0][0], ppid: ppid, pgid: pgid, startTicks: start}, nil
}
