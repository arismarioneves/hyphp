// Package pkgmgr baixa, verifica e extrai runtimes do catálogo de pacotes.
package pkgmgr

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"hyphp/internal/runtime"
)

// CatalogSchema é o único schema de catálogo que este binário entende. Os
// campos só são acrescentados: o catálogo remoto é lido por toda versão
// instalada a partir da 4.0.0.
const CatalogSchema = 1

// KindCACert é o pacote de certificados de CA da Mozilla (curl.se) que o PHP
// do Windows usa no HTTPS de saída. Não é runtime: não vai para bin/ nem
// aparece na tela Runtimes.
const KindCACert runtime.Kind = "cacert"

// knownKinds são os tipos que este binário sabe usar. Um catálogo remoto mais
// novo pode trazer tipos que uma versão antiga não conhece: esses pacotes são
// descartados na leitura, e o resto do catálogo vale.
var knownKinds = []runtime.Kind{
	runtime.PHP, runtime.Apache, runtime.Nginx, runtime.MySQL, runtime.MariaDB,
	runtime.Mailpit, runtime.Mkcert, runtime.PhpMyAdmin, KindCACert,
}

var hexSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Package struct {
	ID       string       `json:"id"` // "php-8.3.33-nts-vs16-x64"
	Kind     runtime.Kind `json:"kind"`
	Version  string       `json:"version"`
	URL      string       `json:"url"`
	SHA256   string       `json:"sha256"` // hex minúsculo, obrigatório
	Compiler string       `json:"compiler"`
	Arch     string       `json:"arch"`
	Notes    string       `json:"notes"`
	// Mirrors são URLs tentadas em ordem quando URL falha (arquivo removido na
	// origem, HTML no lugar do arquivo, sha256 diferente, conexão caída). O
	// sha256 é o mesmo: um mirror nunca entrega outro arquivo.
	Mirrors []string `json:"mirrors,omitempty"`
	// Formula é o nome de instalação no Homebrew ("shivammathur/php/php@8.3");
	// vazio no catálogo. Os pacotes do Mac montados a partir de brew.Formulas
	// usam este campo em vez de URL, e a UI mostra `brew install <formula>` no
	// lugar do link.
	Formula string `json:"formula,omitempty"`
}

type Catalog struct {
	Schema int `json:"schema"`
	// Serial só sobe (AAAAMMDDNN): um catálogo remoto só substitui o que está
	// em uso se o serial for maior, então um catálogo antigo servido de novo não
	// desfaz uma correção de link.
	Serial    int64     `json:"serial"`
	UpdatedAt string    `json:"updatedAt"`
	Packages  []Package `json:"packages"`
}

//go:embed catalog.json
var embeddedCatalog []byte

// LoadEmbedded decodifica e valida o catalog.json compilado no binário.
func LoadEmbedded() (Catalog, error) {
	return ParseCatalog(embeddedCatalog)
}

// ParseCatalog decodifica e valida um catalog.json, embutido ou remoto. Os
// pacotes de tipo desconhecido saem antes da validação.
func ParseCatalog(raw []byte) (Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(raw, &c); err != nil {
		return Catalog{}, fmt.Errorf("pkgmgr: catalog.json inválido: %w", err)
	}
	c.Packages = slices.DeleteFunc(c.Packages, func(p Package) bool { return !slices.Contains(knownKinds, p.Kind) })
	if err := c.Validate(); err != nil {
		return Catalog{}, err
	}
	return c, nil
}

// Validate aplica as regras que todo catálogo cumpre antes de o app usá-lo: o
// embutido no teste, o remoto antes de substituir o em uso e o do repositório
// antes de o hyphp-release assiná-lo.
func (c Catalog) Validate() error {
	if c.Schema != CatalogSchema {
		return fmt.Errorf("pkgmgr: catálogo com schema %d; este binário entende %d", c.Schema, CatalogSchema)
	}
	if c.Serial <= 0 {
		return errors.New("pkgmgr: catálogo sem serial")
	}
	if len(c.Packages) == 0 {
		return errors.New("pkgmgr: catálogo sem pacotes")
	}
	ids := make(map[string]bool, len(c.Packages))
	for _, p := range c.Packages {
		if err := p.validate(); err != nil {
			return err
		}
		if ids[p.ID] {
			return fmt.Errorf("pkgmgr: pacote %q repetido no catálogo", p.ID)
		}
		ids[p.ID] = true
	}
	return nil
}

// validate exige sha256 em todo pacote: só o hash garante que o arquivo
// baixado é o que foi conferido ao montar o catálogo.
func (p Package) validate() error {
	switch {
	case p.ID == "":
		return errors.New("pkgmgr: pacote sem id")
	case !slices.Contains(knownKinds, p.Kind):
		return fmt.Errorf("pkgmgr: %s: tipo %q desconhecido", p.ID, p.Kind)
	case p.Version == "" || !strings.Contains(p.ID, p.Version):
		return fmt.Errorf("pkgmgr: %s: o id tem de conter a versão %q", p.ID, p.Version)
	case !hexSHA256.MatchString(p.SHA256):
		return fmt.Errorf("pkgmgr: %s: sha256 %q não tem 64 hex minúsculos", p.ID, p.SHA256)
	case p.Kind == runtime.Mkcert && !strings.HasSuffix(p.URL, ".exe"):
		return fmt.Errorf("pkgmgr: %s: o mkcert é distribuído como .exe solto", p.ID)
	}
	for _, u := range p.sources() {
		if !strings.HasPrefix(u, "https://") {
			return fmt.Errorf("pkgmgr: %s: url %q não é https", p.ID, u)
		}
	}
	return nil
}

// sources são as URLs do pacote na ordem de tentativa.
func (p Package) sources() []string {
	return append([]string{p.URL}, p.Mirrors...)
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
