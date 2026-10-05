package render

import (
	"bytes"
	"fmt"
	"path/filepath"
)

// RenderMyIni gera o my.ini do MySQL ou do MariaDB embutido. Determinístico.
//
// baseDir é a pasta do runtime (runtime.Installed.Dir), dataDir é o datadir
// do motor (var/mysql-data ou var/mariadb-data) e logDir é o <log> global.
// Todos absolutos; a normalização para "/" acontece aqui porque o servidor no
// Windows aceita "/" e rejeita "\" não escapado num arquivo de opções.
//
// ipv6Loopback diz se a máquina tem ::1 (netcfg.IPv6Loopback). Com ele, o
// servidor escuta nos dois loopbacks: o Windows resolve "localhost" para ::1
// primeiro, e com o MySQL só em 127.0.0.1 cada conexão a "localhost" espera
// ~2 s até o Windows desistir do ::1 (ele repete a tentativa recusada antes
// de falhar). É o host que a maioria dos projetos PHP usa. Continua fora da
// rede: são só os dois loopbacks. Sem ::1 na máquina a lista não pode tê-lo,
// porque o servidor aborta se não consegue escutar em um dos endereços. O
// MariaDB aceita a lista a partir do 10.11, a versão mais antiga do catálogo.
//
// mariadb tira o que só o MySQL conhece: o MariaDB recusa opção desconhecida
// e não sobe com `mysqlx`.
//
// socket é o socket Unix do HyPHP (MySQLSocketPath), o mesmo que o php.ini
// usa nos *.default_socket; "" não emite a linha (Windows). Vai também em
// [client] para o mysql do keg achar o servidor sem --socket. log-error e
// open_files_limit dependem do SO (myini_windows.go / myini_darwin.go).
func RenderMyIni(port int, baseDir, dataDir, logDir, socket string, ipv6Loopback, mariadb bool) []byte {
	logName := "mysql.log"
	if mariadb {
		logName = "mariadb.log"
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Gerado pelo HyPHP — não editar à mão.\n")
	fmt.Fprintf(&b, "# Fonte: internal/render/myini.go — reescrito a cada Reconcile.\n\n")

	fmt.Fprintf(&b, "[mysqld]\n")
	fmt.Fprintf(&b, "port = %d\n", port)
	fmt.Fprintf(&b, "basedir = %q\n", SlashDir(baseDir))
	fmt.Fprintf(&b, "datadir = %q\n", SlashDir(dataDir))
	if socket != "" {
		fmt.Fprintf(&b, "socket = %q\n", filepath.ToSlash(socket))
	}
	if myIniLogError {
		fmt.Fprintf(&b, "log-error = %q\n", SlashDir(logDir)+"/"+logName)
	}
	fmt.Fprintf(&b, "character-set-server = utf8mb4\n")
	fmt.Fprintf(&b, "collation-server = utf8mb4_unicode_ci\n")
	fmt.Fprintf(&b, "max_connections = 100\n")
	if myIniOpenFilesLimit > 0 {
		fmt.Fprintf(&b, "open_files_limit = %d\n", myIniOpenFilesLimit)
	}
	fmt.Fprintf(&b, "innodb_buffer_pool_size = 128M\n")
	fmt.Fprintf(&b, "skip-log-bin\n")
	bind := "127.0.0.1"
	if ipv6Loopback {
		bind += ",::1"
	}
	fmt.Fprintf(&b, "bind-address = %s\n", bind)
	if !mariadb {
		// X Protocol desligado: o HyPHP só fala o protocolo clássico, e o
		// plugin ignora o bind-address — escutava em 0.0.0.0:33060 e
		// [::]:33060, com o root sem senha, a um clique no aviso do firewall
		// de ficar exposto na rede. O MariaDB não tem X Protocol.
		fmt.Fprintf(&b, "mysqlx = OFF\n")
	}

	fmt.Fprintf(&b, "\n[client]\n")
	fmt.Fprintf(&b, "port = %d\n", port)
	if socket != "" {
		fmt.Fprintf(&b, "socket = %q\n", filepath.ToSlash(socket))
	}

	return b.Bytes()
}
