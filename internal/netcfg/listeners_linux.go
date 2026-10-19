package netcfg

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// listeningPorts devolve porta → PID de todo listener TCP (v4 e v6) cujo dono
// dá para ver. O /proc/net/tcp{,6} traz porta e inode de cada socket; o PID
// sai de /proc/<pid>/fd, que só é legível para os processos do próprio
// usuário. Listener de outro usuário fica de fora, como no lsof sem root do
// Mac: o bind de IsFree continua vendo a porta ocupada.
func listeningPorts() (map[int]int, error) {
	inodes := map[uint64]int{}
	for _, p := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		raw, err := os.ReadFile(p)
		if err != nil {
			// Sem IPv6 no kernel o tcp6 não existe; o tcp sempre existe.
			if p == "/proc/net/tcp6" && errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("ler %s: %w", p, err)
		}
		for inode, port := range parseProcNetTCP(string(raw)) {
			inodes[inode] = port
		}
	}
	res := map[int]int{}
	if len(inodes) == 0 {
		return res, nil
	}
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("ler /proc: %w", err)
	}
	for _, d := range procs {
		pid, err := strconv.Atoi(d.Name())
		if err != nil || pid <= 0 {
			continue
		}
		fdDir := "/proc/" + d.Name() + "/fd"
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(fdDir + "/" + fd.Name())
			if err != nil {
				continue
			}
			num, ok := strings.CutPrefix(link, "socket:[")
			if !ok {
				continue
			}
			inode, err := strconv.ParseUint(strings.TrimSuffix(num, "]"), 10, 64)
			if err != nil {
				continue
			}
			if port, ok := inodes[inode]; ok {
				if _, seen := res[port]; !seen {
					res[port] = pid
				}
			}
		}
	}
	return res, nil
}

// parseProcNetTCP extrai inode → porta dos sockets em LISTEN (estado 0A) de
// um /proc/net/tcp ou tcp6. O endereço local é "<ip hex>:<porta hex>"; o
// inode é o décimo campo.
func parseProcNetTCP(raw string) map[uint64]int {
	res := map[uint64]int{}
	for _, line := range strings.Split(raw, "\n") {
		f := strings.Fields(line)
		if len(f) < 10 || f[3] != "0A" {
			continue
		}
		_, portHex, ok := strings.Cut(f[1], ":")
		if !ok {
			continue
		}
		port, err := strconv.ParseUint(portHex, 16, 16)
		if err != nil || port == 0 {
			continue
		}
		inode, err := strconv.ParseUint(f[9], 10, 64)
		if err != nil || inode == 0 {
			continue
		}
		res[inode] = int(port)
	}
	return res
}

// processImagePath devolve o executável do PID por /proc/<pid>/exe, que já
// vem com os symlinks resolvidos.
func processImagePath(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("pid inválido: %d", pid)
	}
	exe, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if err != nil {
		return "", fmt.Errorf("/proc/%d/exe: %w", pid, err)
	}
	return exe, nil
}
