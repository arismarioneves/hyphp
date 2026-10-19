package sysinfo

import (
	"os"
	"strconv"
	"strings"
)

// OS devolve o nome da distribuição, como "Ubuntu 24.04.1 LTS", lido do
// os-release (em /etc, com /usr/lib de reserva, como manda o padrão).
func OS() string {
	for _, p := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		if raw, err := os.ReadFile(p); err == nil {
			return osRelease(string(raw))
		}
	}
	return "Linux"
}

// osRelease escolhe PRETTY_NAME; sem ele, NAME e VERSION_ID; sem nada, "Linux".
// Os valores podem vir entre aspas, com escapes de shell.
func osRelease(raw string) string {
	campos := map[string]string{}
	for _, linha := range strings.Split(raw, "\n") {
		chave, valor, ok := strings.Cut(strings.TrimSpace(linha), "=")
		if !ok || strings.HasPrefix(chave, "#") {
			continue
		}
		if v, err := strconv.Unquote(valor); err == nil {
			valor = v
		} else {
			valor = strings.Trim(valor, `'"`)
		}
		campos[chave] = valor
	}
	if v := campos["PRETTY_NAME"]; v != "" {
		return v
	}
	if n := campos["NAME"]; n != "" {
		return strings.TrimSpace(n + " " + campos["VERSION_ID"])
	}
	return "Linux"
}
