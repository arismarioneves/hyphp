package pkgmgr

import (
	"regexp"
	"strings"
	"testing"

	"hyphp/internal/runtime"
)

var hexSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// O catálogo é editado à mão; um typo no JSON ou num hash quebraria todo download.
func TestLoadEmbedded(t *testing.T) {
	c, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	if c.UpdatedAt == "" || len(c.Packages) == 0 {
		t.Fatalf("catálogo vazio: %+v", c)
	}
	known := map[runtime.Kind]bool{runtime.PHP: true, runtime.Apache: true, runtime.Nginx: true, runtime.MySQL: true, runtime.Mailpit: true, runtime.Mkcert: true, runtime.PhpMyAdmin: true}
	ids := map[string]bool{}
	for _, p := range c.Packages {
		t.Run(p.ID, func(t *testing.T) {
			if p.ID == "" || ids[p.ID] {
				t.Errorf("id vazio ou duplicado: %q", p.ID)
			}
			ids[p.ID] = true
			if !known[p.Kind] {
				t.Errorf("kind desconhecido %q", p.Kind)
			}
			if p.Version == "" || !strings.Contains(p.ID, p.Version) {
				t.Errorf("id %q deve conter a versão %q", p.ID, p.Version)
			}
			if !strings.HasPrefix(p.URL, "https://") {
				t.Errorf("url deve ser https: %q", p.URL)
			}
			if p.SHA256 != "" && !hexSHA256.MatchString(p.SHA256) {
				t.Errorf("sha256 deve ser hex minúsculo de 64 chars ou vazio: %q", p.SHA256)
			}
			if p.SHA256 == "" && !strings.Contains(strings.ToLower(p.Notes), "verificar manualmente") {
				t.Errorf("sha256 vazio exige notes com 'verificar manualmente'")
			}
			if p.Kind == runtime.Mkcert && !strings.HasSuffix(p.URL, ".exe") {
				t.Errorf("mkcert é distribuído como .exe solto")
			}
		})
	}
}

func TestCatalogLookups(t *testing.T) {
	c, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	php := c.ByKind(runtime.PHP)
	if len(php) < 4 {
		t.Fatalf("esperava ≥4 pacotes PHP, veio %d", len(php))
	}
	if _, ok := c.ByID("mailpit-1.31.1"); !ok {
		t.Fatal("ByID(mailpit-1.31.1) não encontrado")
	}
	if _, ok := c.ByID("nao-existe"); ok {
		t.Fatal("ByID inexistente devolveu ok")
	}
}
