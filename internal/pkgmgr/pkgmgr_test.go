package pkgmgr

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hyphp/internal/runtime"
)

// buildZip cria um zip em memória com as entradas dadas (nome → conteúdo; nome
// terminado em "/" vira diretório).
func buildZip(t *testing.T, entries map[string]string) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range entries {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestZipRoot(t *testing.T) {
	cases := []struct {
		name    string
		entries map[string]string
		want    string
	}{
		{"nginx: raiz única", map[string]string{"nginx-1.30.5/nginx.exe": "x", "nginx-1.30.5/conf/nginx.conf": "x"}, "nginx-1.30.5"},
		{"apache lounge: raiz única + arquivos soltos", map[string]string{"Apache24/bin/httpd.exe": "x", "ReadMe.txt": "x", "-- Win64 VS18  --": "x"}, "Apache24"},
		{"mailpit: só arquivos soltos", map[string]string{"mailpit.exe": "x", "LICENSE": "x", "README.md": "x"}, ""},
		{"php: vários diretórios na raiz", map[string]string{"php.exe": "x", "ext/php_curl.dll": "x", "dev/php8.lib": "x"}, ""},
		{"barras invertidas", map[string]string{`nginx-1.30.5\nginx.exe`: "x", `nginx-1.30.5\conf\nginx.conf`: "x"}, "nginx-1.30.5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := zipRoot(buildZip(t, c.entries)); got != c.want {
				t.Fatalf("zipRoot = %q; want %q", got, c.want)
			}
		})
	}
}

func TestExtractZip(t *testing.T) {
	t.Run("strip da raiz e arquivos soltos preservados", func(t *testing.T) {
		r := buildZip(t, map[string]string{"Apache24/bin/httpd.exe": "MZ", "Apache24/conf/httpd.conf": "#", "ReadMe.txt": "leia"})
		dest := filepath.Join(t.TempDir(), "apache-2.4.68-vs18-x64")
		if err := extractZip(r, dest, "Apache24"); err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{"bin/httpd.exe", "conf/httpd.conf", "ReadMe.txt"} {
			if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(p))); err != nil {
				t.Errorf("faltou %s: %v", p, err)
			}
		}
		if _, err := os.Stat(filepath.Join(dest, "Apache24")); err == nil {
			t.Error("a raiz Apache24/ não deveria ter sido recriada dentro do destino")
		}
	})
	t.Run("sem strip", func(t *testing.T) {
		r := buildZip(t, map[string]string{"php.exe": "MZ", "ext/php_curl.dll": "MZ"})
		dest := filepath.Join(t.TempDir(), "php-8.3.33-nts-vs16-x64")
		if err := extractZip(r, dest, ""); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dest, "ext", "php_curl.dll"))
		if err != nil || string(got) != "MZ" {
			t.Fatalf("ext/php_curl.dll = %q, %v", got, err)
		}
	})
	t.Run("zip-slip rejeitado", func(t *testing.T) {
		parent := t.TempDir()
		dest := filepath.Join(parent, "dest")
		for _, evil := range []string{"../evil.txt", "ok/../../evil.txt", `..\evil.txt`, "/abs/evil.txt", `C:\evil.txt`} {
			r := buildZip(t, map[string]string{"ok/fine.txt": "x", evil: "pwned"})
			err := extractZip(r, dest, "")
			if err == nil || !strings.Contains(err.Error(), "zip-slip") {
				t.Errorf("entrada %q: esperava erro zip-slip, veio %v", evil, err)
			}
			if _, statErr := os.Stat(filepath.Join(parent, "evil.txt")); statErr == nil {
				t.Errorf("entrada %q escreveu fora do destino", evil)
			}
		}
	})
}

func TestVerifySHA256(t *testing.T) {
	cases := []struct {
		name, got, want string
		wantErr         bool
	}{
		{"igual", "abc123", "abc123", false},
		{"apache lounge publica maiúsculas", "f6dcf17d08aa", "F6DCF17D08AA", false},
		{"vazio não verifica", "qualquer", "", false},
		{"diferente", "abc", "abd", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := verifySHA256(c.got, c.want); (err != nil) != c.wantErr {
				t.Fatalf("verifySHA256(%q,%q) = %v; wantErr %v", c.got, c.want, err, c.wantErr)
			}
		})
	}
}

func TestDownloadCalculaSHAEProgresso(t *testing.T) {
	payload := make([]byte, 600*1024) // > 2 × 256 KB → pelo menos 3 chamadas de progresso
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	var calls int
	var lastDone, lastTotal int64
	dst := filepath.Join(t.TempDir(), "x.part")
	got, err := Download(context.Background(), srv.Client(), srv.URL+"/x.zip", dst, func(done, total int64) {
		calls++
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha = %s; want %s", got, hex.EncodeToString(sum[:]))
	}
	if calls < 3 || lastDone != int64(len(payload)) || lastTotal != int64(len(payload)) {
		t.Fatalf("progresso: calls=%d lastDone=%d lastTotal=%d", calls, lastDone, lastTotal)
	}
	if st, err := os.Stat(dst); err != nil || st.Size() != int64(len(payload)) {
		t.Fatalf("arquivo destino: %v %v", st, err)
	}
}

func TestDownloadRejeitaHTML(t *testing.T) {
	// Apache Lounge devolve HTTP 200 text/html "Oops... not found" quando a build foi substituída.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>Oops</html>"))
	}))
	defer srv.Close()
	_, err := Download(context.Background(), srv.Client(), srv.URL+"/httpd.zip", filepath.Join(t.TempDir(), "x"), func(int64, int64) {})
	if err == nil || !strings.Contains(err.Error(), "text/html") {
		t.Fatalf("esperava erro por text/html, veio %v", err)
	}
}

func TestInstallSHAMismatchApagaTmp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte("conteudo qualquer"))
	}))
	defer srv.Close()
	bin, tmp := t.TempDir(), t.TempDir()
	m := NewManager(bin, tmp, srv.Client())
	pkg := Package{ID: "nginx-9.9.9", Kind: runtime.Nginx, Version: "9.9.9", URL: srv.URL + "/nginx.zip", SHA256: strings.Repeat("0", 64)}

	var last Progress
	_, err := m.Install(context.Background(), pkg, func(p Progress) { last = p })
	if err == nil || !strings.Contains(err.Error(), "sha256 divergente") {
		t.Fatalf("esperava erro de SHA, veio %v", err)
	}
	if last.Phase != "error" || last.Error == "" || last.PackageID != pkg.ID {
		t.Fatalf("último Progress = %+v; esperava Phase=error com Error preenchido", last)
	}
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Fatalf("tmp deveria estar vazio, tem %d entradas", len(entries))
	}
	if _, statErr := os.Stat(filepath.Join(bin, "nginx")); statErr == nil {
		t.Fatal("nada deveria ter sido extraído em bin/nginx")
	}
}

// Cancelar no meio do download (runtimes de centenas de MB) tem de parar a
// transferência, avisar a UI com a fase própria — não "erro" — e não deixar o
// .part nem nada em bin/.
func TestInstallCanceladoNoDownload(t *testing.T) {
	chegou := make(chan struct{})
	liberar := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Length", "1000000")
		_, _ = w.Write(make([]byte, 1000))
		w.(http.Flusher).Flush()
		close(chegou)
		select { // segura a resposta até o cliente desistir
		case <-r.Context().Done():
		case <-liberar:
		}
	}))
	defer srv.Close()
	defer close(liberar)
	bin, tmp := t.TempDir(), t.TempDir()
	m := NewManager(bin, tmp, srv.Client())
	pkg := Package{ID: "mysql-9.9.9", Kind: runtime.MySQL, Version: "9.9.9", URL: srv.URL + "/mysql.zip", SHA256: strings.Repeat("0", 64)}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-chegou
		cancel()
	}()
	var last Progress
	_, err := m.Install(ctx, pkg, func(p Progress) { last = p })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("esperava context.Canceled, veio %v", err)
	}
	if last.Phase != PhaseCanceled || last.PackageID != pkg.ID {
		t.Fatalf("último Progress = %+v; esperava Phase=%s", last, PhaseCanceled)
	}
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Fatalf("sobrou download parcial em tmp: %d entradas", len(entries))
	}
	if _, statErr := os.Stat(filepath.Join(bin, "mysql")); statErr == nil {
		t.Fatal("nada deveria ter ido para bin/mysql")
	}
}
