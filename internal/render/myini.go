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
func RenderMyIni(port int, baseDir, dataDir, logDir string) []byte {
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
	fmt.Fprintf(&b, "bind-address = 127.0.0.1\n")

	fmt.Fprintf(&b, "\n[client]\n")
	fmt.Fprintf(&b, "port = %d\n", port)

	return b.Bytes()
}
