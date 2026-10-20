package netcfg

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// UnprivilegedPortStartPath é a menor porta que um processo sem root pode
// abrir. Variável: os testes apontam para um arquivo temporário.
var UnprivilegedPortStartPath = "/proc/sys/net/ipv4/ip_unprivileged_port_start"

// SysctlPortsPath é o arquivo do HyPHP que fixa esse limite a cada boot.
var SysctlPortsPath = "/etc/sysctl.d/60-hyphp.conf"

// sysctlPortsKey é a chave do limite no sysctl.
const sysctlPortsKey = "net.ipv4.ip_unprivileged_port_start"

// UnprivilegedPortStart lê o limite de agora. Ler não pede senha.
func UnprivilegedPortStart() (int, error) {
	raw, err := os.ReadFile(UnprivilegedPortStartPath)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", UnprivilegedPortStartPath, err)
	}
	return n, nil
}

// RenderSysctlPorts devolve o arquivo do sysctl que baixa o limite para start.
func RenderSysctlPorts(start int) string {
	return fmt.Sprintf("%s\n%s = %d\n", ResolverRuleMark, sysctlPortsKey, start)
}

// ParseSysctlPorts lê esse arquivo: ours diz se a primeira linha é a marca do
// HyPHP; start é o limite gravado, ou -1 quando falta.
func ParseSysctlPorts(content string) (ours bool, start int) {
	start = -1
	for i, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if i == 0 && line == ResolverRuleMark {
			ours = true
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != sysctlPortsKey {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			start = n
		}
	}
	return ours, start
}
