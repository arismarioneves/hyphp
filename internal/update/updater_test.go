package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// feed simula a pasta releases/ publicada.
type feed struct {
	srv        *httptest.Server
	latest     []byte
	sig        []byte
	installer  []byte
	installGET atomic.Int32
}

func novoFeed(t *testing.T, priv ed25519.PrivateKey, version string, installer []byte, shaAnunciado string) *feed {
	t.Helper()
	sum := sha256.Sum256(installer)
	if shaAnunciado == "" {
		shaAnunciado = hex.EncodeToString(sum[:])
	}
	l := Latest{Schema: 1, Release: Release{
		Version: version, Date: "2026-09-26", Notes: []string{"novidade"},
		WindowsAMD64: &Artifact{Path: version + "/setup.exe", Size: int64(len(installer)), SHA256: shaAnunciado},
	}}
	body, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	f := &feed{latest: body, installer: installer}
	f.sig = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, body)) + "\n")
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest.json", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(f.latest) })
	mux.HandleFunc("/releases/latest.json.sig", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(f.sig) })
	mux.HandleFunc("/releases/"+version+"/setup.exe", func(w http.ResponseWriter, _ *http.Request) {
		f.installGET.Add(1)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(f.installer)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func novoUpdater(t *testing.T, f *feed, pub ed25519.PublicKey, dir string) (*Updater, *[]Status) {
	t.Helper()
	var vistos []Status
	u := New(Config{
		Current:   "1.0.0",
		URL:       f.srv.URL + "/releases/latest.json",
		Dir:       dir,
		Key:       pub,
		Client:    f.srv.Client(),
		Enabled:   true,
		AutoCheck: func() bool { return true },
		OnStatus:  func(s Status) { vistos = append(vistos, s) },
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return u, &vistos
}

func TestCheckBaixaVersaoNovaEFicaPronto(t *testing.T) {
	pub, priv := chaves(t)
	f := novoFeed(t, priv, "1.1.0", []byte("instalador 1.1.0"), "")
	dir := t.TempDir()
	u, _ := novoUpdater(t, f, pub, dir)

	if err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := u.Status()
	if s.State != StateReady || s.Available != "1.1.0" || len(s.Notes) != 1 {
		t.Fatalf("status %+v", s)
	}
	got, err := os.ReadFile(filepath.Join(dir, "1.1.0", "setup.exe"))
	if err != nil || string(got) != "instalador 1.1.0" {
		t.Fatalf("instalador não ficou em Dir/<versão>/: %v", err)
	}
}

// Um sha divergente é instalador adulterado ou corrompido: não pode sobrar
// nada que o próximo boot confunda com arquivo verificado.
func TestCheckShaDivergenteNaoDeixaArquivo(t *testing.T) {
	pub, priv := chaves(t)
	f := novoFeed(t, priv, "1.1.0", []byte("instalador 1.1.0"), strings.Repeat("0", 64))
	dir := t.TempDir()
	u, _ := novoUpdater(t, f, pub, dir)

	if err := u.Check(context.Background()); err == nil {
		t.Fatal("sha divergente aceito")
	}
	if s := u.Status(); s.State != StateFailed || s.Error == "" {
		t.Errorf("status %+v", s)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "1.1.0"))
	if len(entries) != 0 {
		t.Errorf("sobrou arquivo: %v", entries)
	}
}

func TestCheckMesmaVersaoFicaEmDia(t *testing.T) {
	pub, priv := chaves(t)
	f := novoFeed(t, priv, "1.0.0", []byte("x"), "")
	u, _ := novoUpdater(t, f, pub, t.TempDir())
	if err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := u.Status(); s.State != StateUpToDate || s.CheckedAt == "" {
		t.Errorf("status %+v", s)
	}
	if n := f.installGET.Load(); n != 0 {
		t.Errorf("baixou o instalador %d vez(es) sem versão nova", n)
	}
}

// Manifesto que não bate com a chave é tratado como hostil: nem o instalador
// que ele aponta é baixado.
func TestCheckAssinaturaInvalidaNaoBaixa(t *testing.T) {
	pub, _ := chaves(t)
	_, outraPriv := chaves(t)
	f := novoFeed(t, outraPriv, "9.9.9", []byte("malicioso"), "")
	u, _ := novoUpdater(t, f, pub, t.TempDir())
	if err := u.Check(context.Background()); err == nil {
		t.Fatal("assinatura inválida aceita")
	}
	if s := u.Status(); s.State != StateFailed || s.Available != "" {
		t.Errorf("status %+v", s)
	}
	if n := f.installGET.Load(); n != 0 {
		t.Errorf("baixou o instalador de manifesto não assinado")
	}
}

func TestCheckReaproveitaInstaladorJaVerificado(t *testing.T) {
	pub, priv := chaves(t)
	f := novoFeed(t, priv, "1.1.0", []byte("instalador 1.1.0"), "")
	dir := t.TempDir()
	u, _ := novoUpdater(t, f, pub, dir)
	if err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Reinício do app: um Updater novo sobre o mesmo diretório.
	u2, _ := novoUpdater(t, f, pub, dir)
	if err := u2.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := u2.Status(); s.State != StateReady {
		t.Errorf("status %+v", s)
	}
	if n := f.installGET.Load(); n != 1 {
		t.Errorf("instalador baixado %d vezes; o já verificado deveria ser reaproveitado", n)
	}
}

func TestCheckApagaVersoesAntigasBaixadas(t *testing.T) {
	pub, priv := chaves(t)
	f := novoFeed(t, priv, "1.2.0", []byte("instalador 1.2.0"), "")
	dir := t.TempDir()
	velha := filepath.Join(dir, "1.1.0")
	if err := os.MkdirAll(velha, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(velha, "setup.exe"), []byte("velho"), 0o644)
	u, _ := novoUpdater(t, f, pub, dir)
	if err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(velha); !os.IsNotExist(err) {
		t.Error("instalador de versão superada não foi apagado")
	}
}

func escreverResultado(t *testing.T, dir string, r Result) {
	t.Helper()
	raw, _ := json.Marshal(r)
	if err := os.WriteFile(filepath.Join(dir, "resultado.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStartLeResultadoDeSucesso(t *testing.T) {
	pub, priv := chaves(t)
	f := novoFeed(t, priv, "1.0.0", []byte("x"), "")
	dir := t.TempDir()
	escreverResultado(t, dir, Result{From: "0.9.0", To: "1.0.0", OK: true})
	_ = os.MkdirAll(filepath.Join(dir, "1.0.0"), 0o755)
	u, _ := novoUpdater(t, f, pub, dir)

	u.loadResult()
	if s := u.Status(); !strings.Contains(s.Message, "1.0.0") || s.State == StateFailed {
		t.Errorf("status %+v", s)
	}
	if _, err := os.Stat(filepath.Join(dir, "resultado.json")); !os.IsNotExist(err) {
		t.Error("resultado.json lido deveria ser removido (mensagem aparece uma vez)")
	}
	if _, err := os.Stat(filepath.Join(dir, "1.0.0")); !os.IsNotExist(err) {
		t.Error("instalador já aplicado continuou ocupando disco")
	}
}

func TestStartLeResultadoDeFalha(t *testing.T) {
	pub, priv := chaves(t)
	f := novoFeed(t, priv, "1.1.0", []byte("x"), "")
	dir := t.TempDir()
	escreverResultado(t, dir, Result{From: "1.0.0", To: "1.1.0", Error: "atualização cancelada: UAC recusado"})
	u, _ := novoUpdater(t, f, pub, dir)
	u.loadResult()
	if s := u.Status(); s.State != StateFailed || !strings.Contains(s.Error, "UAC") {
		t.Errorf("status %+v", s)
	}
}

// Instalador que diz ter terminado bem mas deixa o binário na versão antiga
// é o começo de um loop de update; tem de aparecer como falha.
func TestStartResultadoOkComVersaoErradaEhFalha(t *testing.T) {
	pub, priv := chaves(t)
	f := novoFeed(t, priv, "1.1.0", []byte("x"), "")
	dir := t.TempDir()
	escreverResultado(t, dir, Result{From: "1.0.0", To: "1.1.0", OK: true})
	u, _ := novoUpdater(t, f, pub, dir) // Current = 1.0.0
	u.loadResult()
	if s := u.Status(); s.State != StateFailed {
		t.Errorf("status %+v", s)
	}
}

func TestBuildDeDevFicaInativo(t *testing.T) {
	pub, priv := chaves(t)
	f := novoFeed(t, priv, "1.1.0", []byte("x"), "")
	u := New(Config{Current: "1.0.0", URL: f.srv.URL + "/releases/latest.json", Dir: t.TempDir(), Key: pub, Client: f.srv.Client(), Enabled: false})
	if s := u.Status(); s.State != StateInactive {
		t.Errorf("status %+v", s)
	}
	if err := u.Check(context.Background()); err == nil {
		t.Error("build de dev verificou update")
	}
}

func TestPrepareExigeProntoERecalculaSha(t *testing.T) {
	pub, priv := chaves(t)
	f := novoFeed(t, priv, "1.1.0", []byte("instalador 1.1.0"), "")
	dir := t.TempDir()
	u, _ := novoUpdater(t, f, pub, dir)
	if _, err := u.Prepare(); err == nil {
		t.Fatal("Prepare sem instalador pronto")
	}
	if err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	req, err := u.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if req.To != "1.1.0" || req.From != "1.0.0" || req.Installer != filepath.Join(dir, "1.1.0", "setup.exe") || req.Result != filepath.Join(dir, "resultado.json") {
		t.Errorf("req %+v", req)
	}
	// Adulterado depois de verificado (tmp é gravável pelo usuário).
	_ = os.WriteFile(req.Installer, []byte("trocado"), 0o644)
	u2, _ := novoUpdater(t, f, pub, dir)
	u2.mu.Lock()
	u2.st.State, u2.st.Available, u2.latest = StateReady, "1.1.0", u.latest
	u2.mu.Unlock()
	if _, err := u2.Prepare(); err == nil {
		t.Error("Prepare aceitou instalador alterado depois da verificação")
	}
}

func TestDefaultURL(t *testing.T) {
	t.Setenv(EnvURL, "")
	if u, err := DefaultURL(); err != nil || u != ManifestURL {
		t.Errorf("sem override: %q %v", u, err)
	}
	for _, ok := range []string{"http://127.0.0.1:8123/releases/latest.json", "http://localhost/x/latest.json", "https://staging.example/latest.json"} {
		t.Setenv(EnvURL, ok)
		if _, err := DefaultURL(); err != nil {
			t.Errorf("%s recusada: %v", ok, err)
		}
	}
	// HTTP fora do loopback deixaria o manifesto trafegar em claro; a
	// assinatura segura o conteúdo, mas não há motivo para aceitar.
	for _, ruim := range []string{"http://ae8.com.br/hyphp/releases/latest.json", "ftp://x/latest.json", "::"} {
		t.Setenv(EnvURL, ruim)
		if _, err := DefaultURL(); err == nil {
			t.Errorf("%s aceita", ruim)
		}
	}
}
