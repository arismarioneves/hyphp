package pkgmgr

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		for _, evil := range []string{"../evil.txt", "ok/../../evil.txt", `..\evil.txt`, "/abs/evil.txt"} {
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

// Uma pasta extraída que Detect não reconhece fica invisível para Scan e para o
// Remover da UI e faz o próximo Instalar falhar com "já existe".
func TestInstallDetectFalhoRemovePastaExtraida(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("nginx-9.9.9/nginx.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("não é executável")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()
	bin, tmp := t.TempDir(), t.TempDir()
	m := NewManager(bin, tmp, srv.Client())
	pkg := Package{ID: "nginx-9.9.9", Kind: runtime.Nginx, Version: "9.9.9", URL: srv.URL + "/nginx.zip"}

	_, err = m.Install(context.Background(), pkg, nil)
	if err == nil || !strings.Contains(err.Error(), "não detectado") {
		t.Fatalf("esperava erro de detecção, veio %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(bin, "nginx", "nginx-9.9.9")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("pasta extraída deveria ter sido removida: %v", statErr)
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

// encurtarStall troca o stallTimeout pelo tempo do teste e o restaura no fim.
func encurtarStall(t *testing.T, d time.Duration) {
	t.Helper()
	old := stallTimeout
	stallTimeout = d
	t.Cleanup(func() { stallTimeout = old })
}

// Conexão que entrega parte do corpo e para sem fechar (Wi-Fi caiu, proxy
// segurando) não pode congelar a barra para sempre: expira como falha — fase
// "error", não "canceled", porque o usuário não cancelou nada.
func TestInstallDownloadParadoExpira(t *testing.T) {
	encurtarStall(t, 200*time.Millisecond)
	fim := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Length", "1000000")
		_, _ = w.Write(make([]byte, 1000))
		w.(http.Flusher).Flush()
		select { // nunca termina a resposta nem fecha a conexão por conta própria
		case <-r.Context().Done():
		case <-fim:
		}
	}))
	defer srv.Close()
	defer close(fim)
	bin, tmp := t.TempDir(), t.TempDir()
	m := NewManager(bin, tmp, srv.Client())
	pkg := Package{ID: "mysql-9.9.9", Kind: runtime.MySQL, Version: "9.9.9", URL: srv.URL + "/mysql.zip"}

	// Sem o watchdog o teste esbarra neste prazo e falha com DeadlineExceeded.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var last Progress
	_, err := m.Install(ctx, pkg, func(p Progress) { last = p })
	if !errors.Is(err, ErrStalled) || errors.Is(err, context.Canceled) {
		t.Fatalf("esperava ErrStalled sem context.Canceled, veio %v", err)
	}
	if last.Phase != PhaseError || !strings.Contains(last.Error, ErrStalled.Error()) {
		t.Fatalf("último Progress = %+v; esperava Phase=%s com o motivo", last, PhaseError)
	}
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Fatalf("sobrou download parcial em tmp: %d entradas", len(entries))
	}
}

// O watchdog mede tempo sem bytes, não duração: um download lento que leva
// várias vezes o stallTimeout, mas nunca para, tem de terminar.
// fakeResetter conta os rearmes e guarda os prazos pedidos.
type fakeResetter struct{ prazos []time.Duration }

func (f *fakeResetter) Reset(d time.Duration) bool {
	f.prazos = append(f.prazos, d)
	return true
}

// leiturasFixas devolve uma sequência pré-definida de (n, err) por Read.
type leiturasFixas struct {
	ns   []int
	errs []error
}

func (l *leiturasFixas) Read(p []byte) (int, error) {
	n, err := l.ns[0], l.errs[0]
	l.ns, l.errs = l.ns[1:], l.errs[1:]
	return n, err
}

// TestIdleReaderRearmaSoComBytes pega um vigia que mede a duração total em vez
// da ociosidade: ele abortaria downloads grandes em conexão lenta mas viva.
// Toda leitura com bytes rearma com o prazo d; leitura sem bytes não rearma.
func TestIdleReaderRearmaSoComBytes(t *testing.T) {
	const d = 7 * time.Second
	fr := &fakeResetter{}
	src := &leiturasFixas{
		ns:   []int{3, 0, 5, 0},
		errs: []error{nil, nil, nil, io.EOF},
	}
	ir := &idleReader{r: src, timer: fr, d: d}
	buf := make([]byte, 16)
	for range 4 {
		_, _ = ir.Read(buf)
	}
	if len(fr.prazos) != 2 {
		t.Fatalf("rearmes = %d, quer 2 (só as leituras com bytes)", len(fr.prazos))
	}
	for _, p := range fr.prazos {
		if p != d {
			t.Fatalf("rearme com %v, quer %v", p, d)
		}
	}
}

