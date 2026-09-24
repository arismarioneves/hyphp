// Package pkgmgr baixa, verifica e extrai runtimes do catálogo embutido.
package pkgmgr

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"hyphp/internal/runtime"
)

type Package struct {
	ID       string       `json:"id"` // "php-8.3.33-nts-vs16-x64"
	Kind     runtime.Kind `json:"kind"`
	Version  string       `json:"version"`
	URL      string       `json:"url"`
	SHA256   string       `json:"sha256"` // hex minúsculo; "" = não verificar (evitar)
	Compiler string       `json:"compiler"`
	Arch     string       `json:"arch"`
	Notes    string       `json:"notes"`
}

type Catalog struct {
	UpdatedAt string    `json:"updatedAt"`
	Packages  []Package `json:"packages"`
}

//go:embed catalog.json
var embeddedCatalog []byte

// LoadEmbedded decodifica o catalog.json compilado no binário.
func LoadEmbedded() (Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(embeddedCatalog, &c); err != nil {
		return Catalog{}, fmt.Errorf("pkgmgr: catalog.json inválido: %w", err)
	}
	return c, nil
}

// ByKind filtra pacotes por kind preservando a ordem do catálogo.
func (c Catalog) ByKind(k runtime.Kind) []Package {
	var out []Package
	for _, p := range c.Packages {
		if p.Kind == k {
			out = append(out, p)
		}
	}
	return out
}

// ByID devolve o pacote com o ID dado.
func (c Catalog) ByID(id string) (Package, bool) {
	for _, p := range c.Packages {
		if p.ID == id {
			return p, true
		}
	}
	return Package{}, false
}
