package stack

import (
	"net"
	"strconv"

	"hyphp/internal/render"
	"hyphp/internal/runtime"
	"hyphp/internal/webserver"
)

// phpConfFiles: no Mac o php-fpm.conf vai no mesmo WriteFiles do php.ini,
// porque a varredura do WriteFiles apagaria um arquivo gravado à parte. Assim o
// "mudou" do conjunto cobre também porta e PoolSize, e o iniChanged da série
// reinicia o master quando só os workers mudam.
func phpConfFiles(ini []byte, pool webserver.PHPPool, poolSize int, logDir string) map[string][]byte {
	return map[string][]byte{
		"php.ini":      ini,
		"php-fpm.conf": render.RenderFPMConf(pool.Version, pool.Ports[0], poolSize, logDir),
	}
}

// phpSendmail: no macOS mail() ignora SMTP e chama sendmail_path pelo /bin/sh.
// Com Mailpit, o sendmail dele entrega no SMTP local (caminho entre aspas para
// o shell). Sem Mailpit, /usr/bin/false: mail() falha em vez de cair no
// /usr/sbin/sendmail do sistema, que pode entregar de verdade via Postfix.
func phpSendmail(rts []runtime.Installed, smtpPort int) string {
	mp, ok := runtime.Newest(rts, runtime.Mailpit)
	if !ok {
		return "/usr/bin/false"
	}
	return `"` + mp.Exe + `" sendmail -S ` + net.JoinHostPort("127.0.0.1", strconv.Itoa(smtpPort))
}

// phpMySQLSocket é o socket do mysqld do HyPHP, o mesmo do my.ini.
func phpMySQLSocket(varDir string) string { return render.MySQLSocketPath(varDir) }
