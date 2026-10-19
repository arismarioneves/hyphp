package i18n

import (
	"os"
	"slices"
	"strings"
)

// systemLanguages segue a ordem do gettext: LANGUAGE (lista separada por ':')
// e depois LC_ALL, LC_MESSAGES e LANG. "C" e "POSIX" não nomeiam idioma e
// ficam de fora.
func systemLanguages() []string {
	var langs []string
	add := func(v string) {
		if v == "" || v == "C" || v == "POSIX" || strings.HasPrefix(v, "C.") || slices.Contains(langs, v) {
			return
		}
		langs = append(langs, v)
	}
	for _, v := range strings.Split(os.Getenv("LANGUAGE"), ":") {
		add(v)
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		add(os.Getenv(k))
	}
	return langs
}
