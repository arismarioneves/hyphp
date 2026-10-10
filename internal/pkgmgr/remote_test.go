package pkgmgr

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func chaveDeTeste(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func assinar(priv ed25519.PrivateKey, body []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, body)) + "\n")
}

// fonteDeTeste parte de um "embutido" com serial 100.
func fonteDeTeste(t *testing.T, pub ed25519.PublicKey) (*Source, string) {
	t.Helper()
	emb, err := ParseCatalog([]byte(catalogoJSON(100, pacotePHP)))
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(t.TempDir(), "catalogo")
	return NewSource(emb, pub, cache), cache
}

// O catálogo remoto assinado e mais novo passa a valer e fica guardado para a
// próxima abertura; o mesmo catálogo de novo não é mudança.
func TestCatalogoRemotoAceito(t *testing.T) {
	pub, priv := chaveDeTeste(t)
	s, cache := fonteDeTeste(t, pub)
	body := []byte(catalogoJSON(101, pacotePHP))
	trocou, err := s.Accept(body, assinar(priv, body))
	if err != nil || !trocou || s.Current().Serial != 101 {
		t.Fatalf("aceitar: trocou=%v err=%v serial=%d", trocou, err, s.Current().Serial)
	}
	if got, _ := os.ReadFile(filepath.Join(cache, "catalog.json")); !bytes.Equal(got, body) {
		t.Error("o catálogo aceito não ficou guardado")
	}
	if trocou, err := s.Accept(body, assinar(priv, body)); err != nil || trocou {
		t.Errorf("o mesmo catálogo de novo: trocou=%v err=%v", trocou, err)
	}
}

// Assinatura de outra chave, catálogo inválido ou serial menor: o catálogo em
// uso não muda e nada é guardado.
func TestCatalogoRemotoRecusado(t *testing.T) {
	pub, priv := chaveDeTeste(t)
	_, outra := chaveDeTeste(t)
	novo := []byte(catalogoJSON(101, pacotePHP))
	velho := []byte(catalogoJSON(99, pacotePHP))
	quebrado := []byte(`{"schema":2,"serial":102,"packages":[]}`)
	for _, c := range []struct {
		nome      string
		body, sig []byte
	}{
		{"assinatura de outra chave", novo, assinar(outra, novo)},
		{"assinatura de outro conteúdo", novo, assinar(priv, velho)},
		{"catálogo inválido", quebrado, assinar(priv, quebrado)},
		{"serial menor", velho, assinar(priv, velho)},
	} {
		t.Run(c.nome, func(t *testing.T) {
			s, cache := fonteDeTeste(t, pub)
			if trocou, err := s.Accept(c.body, c.sig); err == nil || trocou {
				t.Fatalf("aceito: trocou=%v err=%v", trocou, err)
			}
			if s.Current().Serial != 100 {
				t.Errorf("catálogo em uso mudou para o serial %d", s.Current().Serial)
			}
			if _, err := os.Stat(filepath.Join(cache, "catalog.json")); !os.IsNotExist(err) {
				t.Error("catálogo recusado guardado")
			}
		})
	}
}

// O guardado é reconferido ao abrir: um arquivo de var/ mexido depois de
// gravado não vale, e um guardado mais velho que o embutido (app atualizado)
// é ignorado sem erro.
func TestCatalogoGuardadoReconferido(t *testing.T) {
	pub, priv := chaveDeTeste(t)
	s, cache := fonteDeTeste(t, pub)
	body := []byte(catalogoJSON(101, pacotePHP))
	if _, err := s.Accept(body, assinar(priv, body)); err != nil {
		t.Fatal(err)
	}

	nova, _ := fonteDeTeste(t, pub)
	nova.cacheDir = cache
	if err := nova.LoadCache(); err != nil || nova.Current().Serial != 101 {
		t.Fatalf("guardado válido: serial %d, err %v", nova.Current().Serial, err)
	}

	mexido := bytes.Replace(body, []byte("8.3.35"), []byte("8.3.36"), -1)
	if err := os.WriteFile(filepath.Join(cache, "catalog.json"), mexido, 0o644); err != nil {
		t.Fatal(err)
	}
	outra, _ := fonteDeTeste(t, pub)
	outra.cacheDir = cache
	if err := outra.LoadCache(); err == nil || outra.Current().Serial != 100 {
		t.Errorf("guardado mexido: serial %d, err %v", outra.Current().Serial, err)
	}

	velho := []byte(catalogoJSON(99, pacotePHP))
	_ = os.WriteFile(filepath.Join(cache, "catalog.json"), velho, 0o644)
	_ = os.WriteFile(filepath.Join(cache, "catalog.json.sig"), assinar(priv, velho), 0o644)
	antigo, _ := fonteDeTeste(t, pub)
	antigo.cacheDir = cache
	if err := antigo.LoadCache(); err != nil || antigo.Current().Serial != 100 {
		t.Errorf("guardado mais velho que o embutido: serial %d, err %v", antigo.Current().Serial, err)
	}
}

// O catálogo vem por HTTP com limite de tamanho; acima dele é recusado antes
// de qualquer leitura do conteúdo.
func TestFetchDoCatalogo(t *testing.T) {
	pub, priv := chaveDeTeste(t)
	body := []byte(catalogoJSON(101, pacotePHP))
	grande := append([]byte(catalogoJSON(102, pacotePHP)), bytes.Repeat([]byte(" "), catalogLimit)...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok/catalog.json":
			_, _ = w.Write(body)
		case "/ok/catalog.json.sig":
			_, _ = w.Write(assinar(priv, body))
		case "/grande/catalog.json":
			_, _ = w.Write(grande)
		case "/grande/catalog.json.sig":
			_, _ = w.Write(assinar(priv, grande))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	s, _ := fonteDeTeste(t, pub)
	if trocou, err := s.Fetch(context.Background(), srv.Client(), srv.URL+"/ok/catalog.json"); err != nil || !trocou {
		t.Fatalf("fetch: trocou=%v err=%v", trocou, err)
	}
	if _, err := s.Fetch(context.Background(), srv.Client(), srv.URL+"/grande/catalog.json"); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Errorf("catálogo acima do limite: %v", err)
	}
}

// A URL de teste nunca abre brecha: fora do loopback só https.
func TestURLDoCatalogo(t *testing.T) {
	t.Setenv(EnvCatalogURL, "")
	if u, err := DefaultCatalogURL(); err != nil || u != CatalogURL {
		t.Errorf("sem override: %q %v", u, err)
	}
	for raw, ok := range map[string]bool{
		"https://exemplo.test/catalog.json":  true,
		"http://127.0.0.1:8080/catalog.json": true,
		"http://exemplo.test/catalog.json":   false,
		"ftp://127.0.0.1/catalog.json":       false,
	} {
		t.Setenv(EnvCatalogURL, raw)
		if _, err := DefaultCatalogURL(); (err == nil) != ok {
			t.Errorf("%s: err = %v, quer aceito=%v", raw, err, ok)
		}
	}
}
