package netcfg

import (
	"fmt"
	"strconv"
	"strings"
)

// ResolverRuleMark é a primeira linha do /etc/resolver/test gravado pelo
// HyPHP. Sem ela o arquivo é de outro programa (Laravel Valet, dnsmasq do
// usuário): o HyPHP não o conta como seu nem o apaga no "Remover do sistema".
const ResolverRuleMark = "# hyphp"

// RenderResolverRule devolve o /etc/resolver/test que manda as consultas .test
// do macOS para o resolvedor do HyPHP em 127.0.0.1:port.
func RenderResolverRule(port int) string {
	return fmt.Sprintf("%s\nnameserver 127.0.0.1\nport %d\n", ResolverRuleMark, port)
}

// ParseResolverRule lê um /etc/resolver/test: ours diz se a primeira linha é
// a marca do HyPHP; port é a porta declarada, ou 53 (a padrão do resolver(5))
// quando a linha falta.
func ParseResolverRule(content string) (ours bool, port int) {
	port = 53
	for i, line := range strings.Split(content, "\n") {
		f := strings.Fields(line)
		switch {
		case i == 0 && strings.TrimSpace(line) == ResolverRuleMark:
			ours = true
		case len(f) == 2 && f[0] == "port":
			if n, err := strconv.Atoi(f[1]); err == nil {
				port = n
			}
		}
	}
	return ours, port
}
