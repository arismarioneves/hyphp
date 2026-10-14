// Package credits reúne os dados da tela Sobre que não vêm do sistema:
// autoria, licença, apoio e contribuidores.
package credits

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

const (
	Author      = "Arismário Neves"
	AuthorLogin = "arismarioneves"
	// Copyright segue a linha de copyright do LICENSE.
	Copyright  = "© 2026 Arismário Neves"
	License    = "MIT"
	SupportURL = "https://buymeacoffee.com/arismarioneves"
)

// File é a lista de contribuidores, a partir da raiz do repositório. O build
// da release a regrava com os dados do GitHub (hyphp-release -contribuidores);
// a versão do repositório vale para builds locais.
const File = "internal/credits/contributors.json"

//go:embed contributors.json
var contributorsJSON []byte

// Contributors devolve os logins do GitHub, de quem mais contribuiu para quem
// menos.
func Contributors() ([]string, error) {
	var logins []string
	if err := json.Unmarshal(contributorsJSON, &logins); err != nil {
		return nil, fmt.Errorf("credits: ler %s: %w", File, err)
	}
	return logins, nil
}
