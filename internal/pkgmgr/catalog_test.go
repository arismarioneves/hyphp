package pkgmgr

import (
	"fmt"
	"path"
	"strings"
	"testing"

	"hyphp/internal/runtime"
)

// O catálogo é editado à mão e é o mesmo arquivo que o workflow publica como
// catálogo remoto: um typo no JSON ou num hash quebraria todo download.
func TestCatalogoEmbutido(t *testing.T) {
	c, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	if c.Schema != CatalogSchema || c.Serial <= 0 {
		t.Errorf("schema %d, serial %d", c.Schema, c.Serial)
	}
	// O Apache Lounge apaga a build anterior a cada recompilação (issue #7).
	if p, ok := c.ByID("apache-2.4.69-vs18-x64"); !ok || !strings.Contains(p.URL, "261002") {
		t.Errorf("Apache 2.4.69 (build 261002) fora do catálogo: %+v", p)
	}
	if n := len(c.ByKind(KindCACert)); n != 1 {
		t.Errorf("o catálogo precisa de exatamente um pacote de CAs, tem %d", n)
	}
}

// O php.net tira o patch anterior de /releases/ a cada patch novo e o move para
// /releases/archives/ com o mesmo nome. Quem atualiza a url de um PHP e esquece
// o mirror (ou erra o nome) deixa o download quebrar no próximo patch.
func TestPHPDeReleasesTemMirrorEmArchives(t *testing.T) {
	const releases = "https://windows.php.net/downloads/releases/"
	const archives = releases + "archives/"
	c, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range c.ByKind(runtime.PHP) {
		if !strings.HasPrefix(p.URL, releases) || strings.HasPrefix(p.URL, archives) {
			continue
		}
		want := archives + path.Base(p.URL)
		found := false
		for _, m := range p.Mirrors {
			if m == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: falta o mirror %s (mirrors: %v)", p.ID, want, p.Mirrors)
		}
	}
}

func TestCatalogLookups(t *testing.T) {
	c, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	if php := c.ByKind(runtime.PHP); len(php) < 4 {
		t.Fatalf("esperava ≥4 pacotes PHP, veio %d", len(php))
	}
	if _, ok := c.ByID("mailpit-1.31.1"); !ok {
		t.Fatal("ByID(mailpit-1.31.1) não encontrado")
	}
	if _, ok := c.ByID("nao-existe"); ok {
		t.Fatal("ByID inexistente devolveu ok")
	}
}

// catalogoJSON monta um catalog.json com o serial e os pacotes dados.
func catalogoJSON(serial int64, pacotes string) string {
	return fmt.Sprintf(`{"schema":1,"serial":%d,"updatedAt":"2026-10-11","packages":[%s]}`, serial, pacotes)
}

var pacotePHP = `{"id":"php-8.3.35-nts-vs16-x64","kind":"php","version":"8.3.35","url":"https://windows.php.net/php-8.3.35.zip","sha256":"` + strings.Repeat("a", 64) + `"}`

// Um catálogo remoto passa pela mesma validação do embutido: é ele que decide
// o que o app baixa e executa.
func TestCatalogoRecusado(t *testing.T) {
	sha := strings.Repeat("a", 64)
	for _, c := range []struct{ nome, json string }{
		{"schema desconhecido", strings.Replace(catalogoJSON(1, pacotePHP), `"schema":1`, `"schema":2`, 1)},
		{"sem serial", catalogoJSON(0, pacotePHP)},
		{"sem pacotes", catalogoJSON(1, "")},
		{"sem sha256", catalogoJSON(1, `{"id":"php-8.3.35","kind":"php","version":"8.3.35","url":"https://x/php.zip"}`)},
		{"url sem https", catalogoJSON(1, `{"id":"php-8.3.35","kind":"php","version":"8.3.35","url":"http://x/php.zip","sha256":"`+sha+`"}`)},
		{"mirror sem https", catalogoJSON(1, `{"id":"php-8.3.35","kind":"php","version":"8.3.35","url":"https://x/php.zip","mirrors":["http://y/php.zip"],"sha256":"`+sha+`"}`)},
		{"id repetido", catalogoJSON(1, pacotePHP+","+pacotePHP)},
		{"id sem a versão", catalogoJSON(1, `{"id":"php-atual","kind":"php","version":"8.3.35","url":"https://x/php.zip","sha256":"`+sha+`"}`)},
		{"dois pacotes de CAs", catalogoJSON(1, pacotePHP+`,{"id":"cacert-2026-09-25","kind":"cacert","version":"2026-09-25","url":"https://x/c.pem","sha256":"`+sha+`"},{"id":"cacert-2026-10-01","kind":"cacert","version":"2026-10-01","url":"https://x/c2.pem","sha256":"`+sha+`"}`)},
		{"JSON quebrado", `{"schema":1,`},
	} {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := ParseCatalog([]byte(c.json)); err == nil {
				t.Error("catálogo aceito")
			}
		})
	}
}

// Uma versão antiga do app lê o catálogo remoto de uma versão nova: o pacote
// de tipo que ela não conhece fica de fora, e o resto continua valendo.
func TestCatalogoIgnoraTipoDesconhecido(t *testing.T) {
	outro := `{"id":"redis-8.0","kind":"redis","version":"8.0","url":"https://x/r.zip","sha256":"` + strings.Repeat("b", 64) + `"}`
	c, err := ParseCatalog([]byte(catalogoJSON(1, pacotePHP+","+outro)))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Packages) != 1 || c.Packages[0].Kind != runtime.PHP {
		t.Errorf("pacotes = %+v", c.Packages)
	}
}
