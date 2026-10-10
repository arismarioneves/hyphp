package pkgmgr

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CatalogURL é o catálogo publicado pelo workflow catalogo.yml. A URL vai
// gravada em cada binário: trocar o domínio corta o catálogo remoto das
// versões instaladas, que ficam com o embutido.
const CatalogURL = "https://ae8.com.br/hyphp/catalogo/catalog.json"

// EnvCatalogURL troca CatalogURL nos testes. A assinatura continua
// obrigatória, então a variável não abre brecha.
const EnvCatalogURL = "HYPHP_CATALOG_URL"

const (
	catalogLimit    = 1 << 20
	catalogSigLimit = 4 << 10
	// getTimeout é o prazo do GetLimited, o mesmo para o catálogo e para o
	// manifesto do update.
	getTimeout = 30 * time.Second
)

// errOlderCatalog é um catálogo remoto com serial menor que o em uso.
var errOlderCatalog = errors.New("pkgmgr: catálogo mais velho que o em uso")

// DefaultCatalogURL devolve EnvCatalogURL ou CatalogURL.
func DefaultCatalogURL() (string, error) {
	raw := os.Getenv(EnvCatalogURL)
	if raw == "" {
		return CatalogURL, nil
	}
	if err := CheckOverrideURL(raw); err != nil {
		return "", fmt.Errorf("pkgmgr: %s %w", EnvCatalogURL, err)
	}
	return raw, nil
}

// CheckOverrideURL é a regra das variáveis que trocam um endereço de produção
// nos testes (catálogo e update): https com host, ou http só em loopback.
func CheckOverrideURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("inválida: %w", err)
	}
	switch {
	case u.Scheme == "https" && u.Host != "":
		return nil
	case u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"):
		return nil
	}
	return fmt.Errorf("precisa ser https (http só em 127.0.0.1/localhost): %q", raw)
}

// GetLimited faz o GET de url e recusa respostas acima de limit bytes. É o
// caminho dos arquivos pequenos e assinados (latest.json, catalog.json e as
// assinaturas): nada passa de limit antes de ser conferido.
func GetLimited(ctx context.Context, client *http.Client, url string, limit int64, userAgent string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, getTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("buscar %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s respondeu HTTP %d", url, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("ler %s: %w", url, err)
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("%s passa de %d bytes", url, limit)
	}
	return raw, nil
}

// Source é o catálogo em uso: o embutido, até um catálogo remoto assinado e
// mais novo ser aceito. Seguro para uso concorrente.
type Source struct {
	key      ed25519.PublicKey
	cacheDir string // var/catalogo: o último catálogo remoto aceito

	mu  sync.RWMutex
	cur Catalog
}

// NewSource parte do catálogo embutido.
func NewSource(embedded Catalog, key ed25519.PublicKey, cacheDir string) *Source {
	return &Source{key: key, cacheDir: cacheDir, cur: embedded}
}

// Current devolve o catálogo em uso. Quem chama não altera o resultado.
func (s *Source) Current() Catalog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// LoadCache usa o catálogo aceito numa sessão anterior, reconferido como se
// tivesse acabado de chegar. Sem cache, fica o embutido; um cache mais velho
// que o embutido (o app foi atualizado) é ignorado sem erro.
func (s *Source) LoadCache() error {
	body, err := os.ReadFile(filepath.Join(s.cacheDir, "catalog.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(filepath.Join(s.cacheDir, "catalog.json.sig"))
	if err != nil {
		return err
	}
	if _, err := s.accept(body, sig, false); err != nil && !errors.Is(err, errOlderCatalog) {
		return err
	}
	return nil
}

// Accept confere a assinatura dos bytes exatos de body, valida o catálogo e o
// põe em uso se o serial subir, guardando-o em cacheDir. Devolve true quando
// trocou. Serial igual ao em uso não é erro: é o mesmo catálogo de novo.
func (s *Source) Accept(body, sig []byte) (bool, error) {
	return s.accept(body, sig, true)
}

func (s *Source) accept(body, sig []byte, persist bool) (bool, error) {
	if err := VerifySignature(s.key, body, sig); err != nil {
		return false, fmt.Errorf("pkgmgr: catálogo remoto: %w", err)
	}
	c, err := ParseCatalog(body)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case c.Serial == s.cur.Serial:
		return false, nil
	case c.Serial < s.cur.Serial:
		return false, fmt.Errorf("%w: serial %d, em uso %d", errOlderCatalog, c.Serial, s.cur.Serial)
	}
	if persist {
		if err := writeCatalogCache(s.cacheDir, body, sig); err != nil {
			return false, err
		}
	}
	s.cur = c
	return true, nil
}

// writeCatalogCache grava o par por tmp+rename, primeiro a assinatura: um
// corte no meio deixa no máximo um par que não confere, e o LoadCache o
// recusa.
func writeCatalogCache(dir string, body, sig []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range []struct {
		name string
		data []byte
	}{{"catalog.json.sig", sig}, {"catalog.json", body}} {
		tmp := filepath.Join(dir, f.name+".tmp")
		if err := os.WriteFile(tmp, f.data, 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, filepath.Join(dir, f.name)); err != nil {
			return err
		}
	}
	return nil
}

// Fetch busca url e url+".sig" e passa o par para Accept.
func (s *Source) Fetch(ctx context.Context, client *http.Client, url string) (bool, error) {
	body, err := GetLimited(ctx, client, url, catalogLimit, "hyphp")
	if err != nil {
		return false, fmt.Errorf("pkgmgr: %w", err)
	}
	sig, err := GetLimited(ctx, client, url+".sig", catalogSigLimit, "hyphp")
	if err != nil {
		return false, fmt.Errorf("pkgmgr: %w", err)
	}
	return s.Accept(body, sig)
}

// RemoteConfig descreve a busca periódica do catálogo remoto.
type RemoteConfig struct {
	URL        string
	Client     *http.Client
	FirstDelay time.Duration // 0 → 60 s: não disputar rede com o boot, como o updater
	Interval   time.Duration // 0 → 6 h
	OnChange   func(Catalog) // chamado com o catálogo novo em uso
	Logger     *slog.Logger
}

// Run busca o catálogo remoto até ctx terminar. Falha de rede, assinatura ou
// serial só vai para o log: o app segue com o catálogo que tinha.
func (s *Source) Run(ctx context.Context, c RemoteConfig) {
	if c.FirstDelay == 0 {
		c.FirstDelay = 60 * time.Second
	}
	if c.Interval == 0 {
		c.Interval = 6 * time.Hour
	}
	go func() {
		t := time.NewTimer(c.FirstDelay)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			trocou, err := s.Fetch(ctx, c.Client, c.URL)
			// App fechando no meio da busca: o erro é o cancelamento, não vale Warn.
			if ctx.Err() != nil {
				return
			}
			switch {
			case err != nil:
				c.Logger.Warn("catálogo remoto", "err", err)
			case trocou:
				c.Logger.Info("catálogo remoto em uso", "serial", s.Current().Serial)
				if c.OnChange != nil {
					c.OnChange(s.Current())
				}
			}
			t.Reset(c.Interval)
		}
	}()
}
