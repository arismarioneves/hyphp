package render

import (
	"bytes"
	"fmt"
	"net"
	"strconv"
	"strings"

	"hyphp/internal/runtime"
)

// RenderFPMConf gera o php-fpm.conf de uma série: um master com um pool que
// escuta na porta da série. Passado com --fpm-config, deixa de fora o
// php-fpm.conf do Homebrew e o php-fpm.d/www.conf dele (pool na :9000).
//
// pm = ondemand: sem request não há filho, então várias séries paradas não
// gastam memória; maxChildren é o "Workers por versão" da tela, o mesmo teto
// de requests simultâneos que os N php-cgi do Windows dão. Sem user/group: o
// fpm os ignora quando não roda como root, e o HyPHP nunca roda como root.
// clear_env = no mantém o comportamento do php-cgi (o worker herda o ambiente).
func RenderFPMConf(major string, port, maxChildren int, logDir string) []byte {
	log := SlashDir(logDir)
	var b bytes.Buffer
	fmt.Fprintf(&b, "; Gerado pelo HyPHP — não editar à mão.\n")
	fmt.Fprintf(&b, "; Fonte: internal/render/phpfpm_darwin.go — reescrito a cada Reconcile.\n\n")

	fmt.Fprintf(&b, "[global]\n")
	fmt.Fprintf(&b, "; Só reserva: com --force-stderr o log do master vai para o supervisor.\n")
	fmt.Fprintf(&b, "error_log = %q\n", log+"/php-fpm-"+major+".log")
	fmt.Fprintf(&b, "daemonize = no\n")

	fmt.Fprintf(&b, "\n[php%s]\n", strings.ReplaceAll(major, ".", ""))
	fmt.Fprintf(&b, "listen = %s\n", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	fmt.Fprintf(&b, "listen.allowed_clients = 127.0.0.1\n")
	fmt.Fprintf(&b, "pm = ondemand\n")
	fmt.Fprintf(&b, "pm.max_children = %d\n", maxChildren)
	fmt.Fprintf(&b, "pm.process_idle_timeout = 10s\n")
	fmt.Fprintf(&b, "pm.max_requests = 0\n")
	fmt.Fprintf(&b, "catch_workers_output = yes\n")
	// decorate_workers_output só existe a partir do 7.3; o fpm do 7.2 recusa
	// diretiva desconhecida ("unknown entry") e nem sobe.
	if runtime.CompareVersions(major, "7.3") >= 0 {
		fmt.Fprintf(&b, "decorate_workers_output = no\n")
	}
	fmt.Fprintf(&b, "clear_env = no\n")
	// O launchd dá 256 arquivos de soft limit a app de GUI; o fpm sobe o seu.
	fmt.Fprintf(&b, "rlimit_files = 4096\n")
	return b.Bytes()
}
