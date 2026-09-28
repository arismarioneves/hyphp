package render

import (
	"bytes"
	"fmt"
)

// RenderMyIni gera o my.ini do MySQL/MariaDB embutido. Determinístico.
//
// baseDir é a pasta do runtime (runtime.Installed.Dir), dataDir é
// <var>/mysql e logDir é o <log> global. Todos absolutos; a normalização para
// "/" acontece aqui porque o MySQL no Windows aceita "/" e rejeita "\" não
// escapado num arquivo de opções.
//
// ipv6Loopback diz se a máquina tem ::1 (netcfg.IPv6Loopback). Com ele, o
// servidor escuta nos dois loopbacks: o Windows resolve "localhost" para ::1
// primeiro, e com o MySQL só em 127.0.0.1 cada conexão a "localhost" espera
// ~2 s até o Windows desistir do ::1 (ele repete a tentativa recusada antes
// de falhar). É o host que a maioria dos projetos PHP usa. Continua fora da
// rede: são só os dois loopbacks. Sem ::1 na máquina a lista não pode tê-lo,
// porque o mysqld aborta se não consegue escutar em um dos endereços.
func RenderMyIni(port int, baseDir, dataDir, logDir string, ipv6Loopback bool) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Gerado pelo HyPHP — não editar à mão.\n")
	fmt.Fprintf(&b, "# Fonte: internal/render/myini.go — reescrito a cada Reconcile.\n\n")

	fmt.Fprintf(&b, "[mysqld]\n")
	fmt.Fprintf(&b, "port = %d\n", port)
	fmt.Fprintf(&b, "basedir = %q\n", slashDir(baseDir))
	fmt.Fprintf(&b, "datadir = %q\n", slashDir(dataDir))
	fmt.Fprintf(&b, "log-error = %q\n", slashDir(logDir)+"/mysql.log")
	fmt.Fprintf(&b, "character-set-server = utf8mb4\n")
	fmt.Fprintf(&b, "collation-server = utf8mb4_unicode_ci\n")
	fmt.Fprintf(&b, "max_connections = 100\n")
	fmt.Fprintf(&b, "innodb_buffer_pool_size = 128M\n")
	fmt.Fprintf(&b, "skip-log-bin\n")
	bind := "127.0.0.1"
	if ipv6Loopback {
		bind += ",::1"
	}
	fmt.Fprintf(&b, "bind-address = %s\n", bind)

	fmt.Fprintf(&b, "\n[client]\n")
	fmt.Fprintf(&b, "port = %d\n", port)

	return b.Bytes()
}
