package netcfg

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// listeningPorts devolve porta → PID de todo listener TCP (v4 e v6). O
// /proc/net/tcp{,6} traz porta e inode de cada socket; o PID sai de
// /proc/<pid>/fd, que só é legível para os processos do próprio usuário.
// Listener de outro usuário (um apache2 do sistema, como root) entra com PID
// 0: abaixo do ip_unprivileged_port_start o bind de teste dá EACCES e não vê
// a porta ocupada, e só esta tabela vê.
func listeningPorts() (map[int]int, error) {
	inodePort := map[uint64]int{}
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
			inodePort[inode] = port
		}
	}
	if len(inodePort) == 0 {
		return map[int]int{}, nil
	}
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("ler /proc: %w", err)
	}
	inodePID := map[uint64]int{}
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
			if _, ok := inodePort[inode]; ok {
				if _, seen := inodePID[inode]; !seen {
					inodePID[inode] = pid
				}
			}
		}
	}
	return portOwners(inodePort, inodePID), nil
}

// portOwners junta as duas tabelas: cada porta em LISTEN com o PID de um
// socket dela, ou 0 quando nenhum socket da porta tem dono visível. A mesma
// porta em IPv4 e IPv6 fica com o dono que aparecer.
func portOwners(inodePort, inodePID map[uint64]int) map[int]int {
	res := map[int]int{}
	for inode, port := range inodePort {
		pid := inodePID[inode]
		if cur, seen := res[port]; !seen || cur == 0 {
			res[port] = pid
		}
	}
	return res
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
