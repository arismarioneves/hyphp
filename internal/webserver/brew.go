package webserver

import (
	"path/filepath"

	"hyphp/internal/runtime"
)

// BrewPrefix devolve o prefixo do Homebrew (/opt/homebrew) de onde o Apache e o
// nginx do Mac leem o etc/ compartilhado (mime.types, fastcgi_params), que fica
// fora do keg. A detecção sempre preenche inst.Prefix; o fallback existe só
// para um Installed antigo ou montado à mão: com o keg em <prefix>/opt/<formula>
// o prefixo é dois níveis acima. Fica num lugar só para os dois servidores não
// divergirem na regra.
func BrewPrefix(inst runtime.Installed) string {
	if inst.Prefix != "" {
		return filepath.ToSlash(inst.Prefix)
	}
	return filepath.ToSlash(filepath.Dir(filepath.Dir(inst.Dir)))
}
