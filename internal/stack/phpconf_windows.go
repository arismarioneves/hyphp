package stack

import (
	"hyphp/internal/runtime"
	"hyphp/internal/webserver"
)

// phpConfFiles: no Windows etc/php/<série> só tem o php.ini; os workers php-cgi
// recebem porta e flags pela linha de comando.
func phpConfFiles(ini []byte, _ webserver.PHPPool, _ int, _ string) map[string][]byte {
	return map[string][]byte{"php.ini": ini}
}

// phpSendmail: no Windows mail() usa SMTP/smtp_port, que já apontam para o
// Mailpit; sem sendmail_path o php.ini sai igual ao de sempre.
func phpSendmail([]runtime.Installed, int) string { return "" }
