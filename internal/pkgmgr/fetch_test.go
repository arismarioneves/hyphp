package pkgmgr

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type resposta struct {
	tipo  string
	corpo []byte
}

// servidorDePacotes serve cada caminho com o content-type e o corpo dados.
func servidorDePacotes(t *testing.T, rotas map[string]resposta) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res, ok := rotas[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", res.tipo)
		_, _ = w.Write(res.corpo)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func pacoteDeCAs(srv *httptest.Server, conteudo []byte, url string, mirrors ...string) Package {
	sum := sha256.Sum256(conteudo)
	for i, m := range mirrors {
		mirrors[i] = srv.URL + m
	}
	return Package{ID: "cacert-2026-09-25", Kind: KindCACert, Version: "2026-09-25",
		URL: srv.URL + url, Mirrors: mirrors, SHA256: hex.EncodeToString(sum[:])}
}

// O Apache Lounge responde 200 com uma página HTML quando apaga uma build
// (issue #7); um mirror pode entregar outro arquivo. Os dois casos caem para
// a próxima fonte, e só o arquivo com o sha256 certo chega em dest.
func TestFetchCaiParaOMirror(t *testing.T) {
	conteudo := []byte("certificados\n")
	srv := servidorDePacotes(t, map[string]resposta{
		"/removido": {"text/html", []byte("<html>Oops</html>")},
		"/trocado":  {"application/octet-stream", []byte("outro arquivo")},
		"/certo":    {"application/x-pem-file", conteudo},
	})
	m := NewManager(t.TempDir(), t.TempDir(), srv.Client())
	dest := filepath.Join(t.TempDir(), "cacert.pem")
	if err := m.Fetch(context.Background(), pacoteDeCAs(srv, conteudo, "/removido", "/trocado", "/certo"), dest); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dest); !bytes.Equal(got, conteudo) {
		t.Errorf("dest = %q, quer o arquivo do mirror certo", got)
	}
}

// Sem nenhuma fonte boa, o erro cita cada tentativa e nada fica em dest.
func TestFetchSemFonteBoa(t *testing.T) {
	srv := servidorDePacotes(t, map[string]resposta{
		"/removido": {"text/html", []byte("<html>Oops</html>")},
		"/trocado":  {"application/octet-stream", []byte("outro arquivo")},
	})
	m := NewManager(t.TempDir(), t.TempDir(), srv.Client())
	dest := filepath.Join(t.TempDir(), "cacert.pem")
	err := m.Fetch(context.Background(), pacoteDeCAs(srv, []byte("certificados\n"), "/removido", "/trocado"), dest)
	if err == nil || !strings.Contains(err.Error(), "/removido") || !strings.Contains(err.Error(), "/trocado") {
		t.Fatalf("erro = %v, quer as duas tentativas", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("dest gravado sem uma fonte boa")
	}
}

// Cancelar no meio do download encerra na hora: o mirror não é tentado, e o
// erro continua sendo de cancelamento, para a tela Runtimes voltar ao estado
// inicial em vez de mostrar "tentando outra fonte".
func TestFetchCanceladoNaoTentaOMirror(t *testing.T) {
	chegou := make(chan struct{})
	var mirror atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mirror" {
			mirror.Store(true)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", "1000000")
		_, _ = w.Write(make([]byte, 1000))
		w.(http.Flusher).Flush()
		close(chegou)
		<-r.Context().Done() // segura a resposta até o cliente desistir
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-chegou
		cancel()
	}()
	m := NewManager(t.TempDir(), t.TempDir(), srv.Client())
	err := m.Fetch(ctx, pacoteDeCAs(srv, []byte("certificados\n"), "/lento", "/mirror"), filepath.Join(t.TempDir(), "cacert.pem"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v, quer context.Canceled", err)
	}
	if mirror.Load() {
		t.Error("o mirror foi tentado depois do cancelamento")
	}
}
