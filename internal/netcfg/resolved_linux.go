package netcfg

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ResolvedDropInPath é a regra de DNS do .test no Linux: um drop-in do
// systemd-resolved. Variável: os testes apontam para uma pasta temporária.
var ResolvedDropInPath = "/etc/systemd/resolved.conf.d/hyphp.conf"

// ResolvConfPath é o resolv.conf que a glibc lê. Variável, como as outras.
var ResolvConfPath = "/etc/resolv.conf"

// resolvedStubAddr é o endereço do stub do systemd-resolved.
const resolvedStubAddr = "127.0.0.53"

// RenderResolvedDropIn devolve o drop-in que manda as consultas .test para o
// resolvedor do HyPHP em 127.0.0.1:port. O DNS= vale para o sistema todo, e
// por isso o resolvedor responde NXDOMAIN fora do .test: o resolved fica com a
// resposta dos servidores da rede.
func RenderResolvedDropIn(port int) string {
	return fmt.Sprintf("%s\n[Resolve]\nDNS=127.0.0.1:%d\nDomains=~test\n", ResolverRuleMark, port)
}

// ParseResolvedDropIn lê o drop-in: ours diz se a primeira linha é a marca do
// HyPHP; port é a porta do DNS= em 127.0.0.1, ou 0 quando falta.
func ParseResolvedDropIn(content string) (ours bool, port int) {
	for i, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case i == 0 && line == ResolverRuleMark:
			ours = true
		case strings.HasPrefix(line, "DNS=127.0.0.1:"):
			if n, err := strconv.Atoi(strings.TrimPrefix(line, "DNS=127.0.0.1:")); err == nil {
				port = n
			}
		}
	}
	return ours, port
}

// ResolvedStub diz se a glibc consulta o stub do systemd-resolved, que é o
// que dá efeito ao drop-in. Vale o primeiro nameserver: a glibc só passa ao
// seguinte quando o primeiro não responde, e um NXDOMAIN para .test vindo de
// outro servidor encerra a consulta. O resolved no ar com o resolv.conf
// apontando para os servidores da rede (o modo sem stub, ou outro programa no
// controle do arquivo) não vale: as consultas nem passariam por ele.
func ResolvedStub() bool {
	f, err := os.Open(ResolvConfPath)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 && fields[0] == "nameserver" {
			return fields[1] == resolvedStubAddr
		}
	}
	return false
}
