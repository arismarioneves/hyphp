package netcfg

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"

	"hyphp/internal/sysproc"
)

// listeningPorts executa `netstat.exe -ano` e devolve porta → PID de todo listener TCP (v4 e v6).
func listeningPorts() (map[int]int, error) {
	cmd := exec.Command("netstat.exe", "-ano")
	sysproc.Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("netstat: %w", err)
	}
	return parseNetstatListening(string(out)), nil
}

// parseNetstatListening percorre a saída de `netstat -ano` e devolve porta local → PID das
// linhas TCP em escuta. Aceita IPv4 ("0.0.0.0:80") e IPv6 ("[::]:80"); a primeira linha
// de cada porta vence. Cabeçalhos (que variam por idioma) e linhas UDP/ESTABLISHED são ignorados.
//
// O listener é reconhecido pelo endereço remoto vazio, não pelo texto do estado: o netstat
// traduz "LISTENING" (em alemão sai "ABHÖREN") e o mapa ficaria vazio nessas máquinas.
func parseNetstatListening(out string) map[int]int {
	res := make(map[int]int)
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 || fields[0] != "TCP" {
			continue
		}
		if remote := fields[2]; remote != "0.0.0.0:0" && remote != "[::]:0" && remote != "*:*" {
			continue
		}
		local := fields[1]
		colon := strings.LastIndexByte(local, ':')
		if colon < 0 {
			continue
		}
		port, err := strconv.Atoi(local[colon+1:])
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(fields[4])
		if err != nil {
			continue
		}
		if _, seen := res[port]; !seen {
			res[port] = pid
		}
	}
	return res
}

// processImagePath devolve o caminho completo do executável do PID.
func processImagePath(pid int) (string, error) {
	// A interface comum usa int; a API do Windows quer DWORD.
	pid32 := uint32(pid)
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid32)
	if err != nil {
		return "", fmt.Errorf("OpenProcess(%d): %w", pid, err)
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return "", fmt.Errorf("QueryFullProcessImageName(%d): %w", pid, err)
	}
	return windows.UTF16ToString(buf[:size]), nil
}
