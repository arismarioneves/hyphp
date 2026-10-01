package i18n

import (
	"os"
	"os/exec"
	"strings"
)

// systemLanguages no macOS lê a lista de idiomas preferidos do usuário:
// um app aberto pelo Finder não herda LANG, então só o ambiente não basta.
// Se o defaults falhar ou vier vazio, cai em LANG como nos outros Unix.
func systemLanguages() []string {
	if out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
		if langs := parseAppleLanguages(string(out)); len(langs) > 0 {
			return langs
		}
	}
	if l := os.Getenv("LANG"); l != "" {
		return []string{l}
	}
	return nil
}

// parseAppleLanguages lê o array que `defaults read -g AppleLanguages`
// imprime: um item por linha entre "(" e ")", com vírgula e, às vezes, aspas.
func parseAppleLanguages(out string) []string {
	var langs []string
	for _, line := range strings.Split(out, "\n") {
		item := strings.Trim(strings.TrimSpace(line), `",`)
		if item == "" || item == "(" || item == ")" || item == "()" {
			continue
		}
		langs = append(langs, item)
	}
	return langs
}
