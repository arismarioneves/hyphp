package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"hyphp/internal/update"
)

const testRepo = "dono/hyphp"

func ambiente(t *testing.T) (web, inst string, pub ed25519.PublicKey, priv ed25519.PrivateKey) {
	t.Helper()
	web = t.TempDir()
	inst = filepath.Join(t.TempDir(), "hyphp-amd64-installer.exe")
	if err := os.WriteFile(inst, []byte("instalador de teste"), 0o644); err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return web, inst, pub, priv
}

// ghCall é uma chamada ao gh falso. Os arquivos são lidos na hora da chamada,
// porque o Publish apaga a pasta temporária ao sair.
type ghCall struct {
	args  []string
	files map[string][]byte // nome base → conteúdo
}

func fakeGH(calls *[]ghCall, fail error) func(args ...string) error {
	return func(args ...string) error {
		c := ghCall{args: args, files: map[string][]byte{}}
		for _, a := range args {
			if b, err := os.ReadFile(a); err == nil {
				c.files[filepath.Base(a)] = b
			}
		}
		*calls = append(*calls, c)
		return fail
	}
}

func publicar(t *testing.T, web, inst, v string, priv ed25519.PrivateKey, calls *[]ghCall) error {
	t.Helper()
	_, err := Publish(Options{WebDir: web, Repo: testRepo, Installer: inst, Version: v, Date: "2026-09-26", Notes: []string{"nota " + v}, Key: priv, GH: fakeGH(calls, nil)})
	return err
}

func lerIndice(t *testing.T, web string) update.Index {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(web, "releases", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var idx update.Index
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatal(err)
	}
	return idx
}

// O que vai para a release é o que o app instalado lê: precisa passar pelo
// mesmo caminho que ele usa, Verify sobre os bytes enviados e ParseLatest.
func TestPublishCriaReleaseQueOAppAceita(t *testing.T) {
	web, inst, pub, priv := ambiente(t)
	var calls []ghCall
	if err := publicar(t, web, inst, "1.0.0", priv, &calls); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("gh chamado %d vezes", len(calls))
	}
	c := calls[0]
	want := []string{"release", "create", "v1.0.0", "--repo", testRepo, "--verify-tag", "--latest", "--title", "HyPHP 1.0.0"}
	if !slices.Equal(c.args[:len(want)], want) {
		t.Fatalf("args = %q", c.args)
	}
	if got := string(c.files["notas.md"]); got != "- nota 1.0.0\n" {
		t.Errorf("notas da release = %q", got)
	}
	body, sig := c.files["latest.json"], c.files["latest.json.sig"]
	if err := update.Verify(pub, body, sig); err != nil {
		t.Fatalf("app recusaria a assinatura: %v", err)
	}
	l, err := update.ParseLatest(body)
	if err != nil {
		t.Fatalf("app recusaria o manifesto: %v", err)
	}
	exe, ok := c.files[l.WindowsAMD64.Path]
	if !ok || l.WindowsAMD64.Path != "hyphp-1.0.0-windows-amd64-setup.exe" {
		t.Fatalf("o path %q não é o nome de um asset enviado (%v)", l.WindowsAMD64.Path, c.args)
	}
	if int64(len(exe)) != l.WindowsAMD64.Size || string(exe) != "instalador de teste" {
		t.Errorf("asset do instalador não é o arquivo passado")
	}

	idx := lerIndice(t, web)
	if idx.GitHub != testRepo || !reflect.DeepEqual(l.Release, idx.Releases[0]) {
		t.Errorf("index.json não registra a release:\n%+v\n%+v", idx, l.Release)
	}
	// O site deixou de ser o feed: nada de manifesto nem instalador nele.
	for _, sobra := range []string{"latest.json", "latest.json.sig", "1.0.0"} {
		if _, err := os.Stat(filepath.Join(web, "releases", sobra)); !os.IsNotExist(err) {
			t.Errorf("releases/%s gravado no site", sobra)
		}
	}
}

func TestPublishOrdenaPorVersaoNumerica(t *testing.T) {
	web, inst, _, priv := ambiente(t)
	var calls []ghCall
	for _, v := range []string{"1.0.9", "1.0.10"} {
		if err := publicar(t, web, inst, v, priv, &calls); err != nil {
			t.Fatal(err)
		}
	}
	idx := lerIndice(t, web)
	if len(idx.Releases) != 2 || idx.Releases[0].Version != "1.0.10" || idx.Releases[1].Version != "1.0.9" {
		t.Errorf("ordem errada: %+v", idx.Releases)
	}
}

// Release publicada é imutável no GitHub: a mesma versão, ou uma menor que a
// última, é recusada antes de chamar o gh.
func TestPublishRecusaRepublicarOuRegredir(t *testing.T) {
	web, inst, _, priv := ambiente(t)
	var calls []ghCall
	if err := publicar(t, web, inst, "1.1.0", priv, &calls); err != nil {
		t.Fatal(err)
	}
	antes, _ := os.ReadFile(filepath.Join(web, "releases", "index.json"))

	if err := publicar(t, web, inst, "1.1.0", priv, &calls); err == nil {
		t.Error("republicou a mesma versão")
	}
	if err := publicar(t, web, inst, "1.0.5", priv, &calls); err == nil {
		t.Error("publicou versão menor que a última")
	}
	if len(calls) != 1 {
		t.Errorf("gh chamado %d vezes; as recusas não podiam chegar nele", len(calls))
	}
	if depois, _ := os.ReadFile(filepath.Join(web, "releases", "index.json")); string(antes) != string(depois) {
		t.Error("index.json mudou depois de publicações recusadas")
	}
}

// gh falhou (sem login, tag ausente, rede): o site não pode anunciar uma
// versão que não existe no GitHub.
func TestPublishGHFalhouNaoMexeNoSite(t *testing.T) {
	web, inst, _, priv := ambiente(t)
	var calls []ghCall
	_, err := Publish(Options{WebDir: web, Repo: testRepo, Installer: inst, Version: "1.0.0", Date: "2026-09-26", Notes: []string{"x"}, Key: priv, GH: fakeGH(&calls, errors.New("tag v1.0.0 não existe"))})
	if err == nil || !strings.Contains(err.Error(), "tag v1.0.0") {
		t.Fatalf("erro do gh não voltou: %v", err)
	}
	if _, err := os.Stat(filepath.Join(web, "releases")); !os.IsNotExist(err) {
		t.Error("o site ganhou arquivos de uma release que falhou")
	}
}

func TestVersoesConsistentes(t *testing.T) {
	root := t.TempDir()
	escrever := func(cfg, info, nsh string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, "build", "windows", "nsis"), 0o755); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(root, "build", "config.yml"), []byte("version: '3'\ninfo:\n  productName: \"HyPHP\"\n  version: \""+cfg+"\"\n"), 0o644)
		_ = os.WriteFile(filepath.Join(root, "build", "windows", "info.json"), []byte(`{"fixed":{"file_version":"`+info+`"},"info":{"0000":{"ProductVersion":"`+info+`"}}}`), 0o644)
		_ = os.WriteFile(filepath.Join(root, "build", "windows", "nsis", "wails_tools.nsh"), []byte("!ifndef INFO_PRODUCTVERSION\n    !define INFO_PRODUCTVERSION \""+nsh+"\"\n!endif\n"), 0o644)
	}
	escrever("1.0.0", "1.0.0", "1.0.0")
	if err := checkVersions(root, "1.0.0"); err != nil {
		t.Errorf("versões iguais recusadas: %v", err)
	}
	escrever("1.0.0", "0.1.0", "1.0.0")
	if err := checkVersions(root, "1.0.0"); err == nil {
		t.Error("info.json divergente aceito")
	}
	escrever("1.0.1", "1.0.0", "1.0.0")
	if err := checkVersions(root, "1.0.0"); err == nil {
		t.Error("config.yml divergente aceito")
	}
	// O instalador sai com a versão do .nsh em "Aplicativos instalados".
	escrever("1.0.0", "1.0.0", "0.1.0")
	if err := checkVersions(root, "1.0.0"); err == nil {
		t.Error("wails_tools.nsh divergente aceito")
	}
}

// O site é HTML puro e roda aberto do disco, onde fetch() de arquivo local é
// bloqueado; ele lê as versões do releases.js. O conteúdo tem de ser o mesmo
// do index.json, senão o site mostra uma versão e o app baixa outra.
func TestReleasesJSEspelhaOIndice(t *testing.T) {
	web, inst, _, priv := ambiente(t)
	var calls []ghCall
	for _, v := range []string{"1.0.0", "1.1.0"} {
		if err := publicar(t, web, inst, v, priv, &calls); err != nil {
			t.Fatal(err)
		}
	}
	lerJS := func() update.Index {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(web, "releases", "releases.js"))
		if err != nil {
			t.Fatal(err)
		}
		s := string(raw)
		if !strings.HasPrefix(s, releasesJSPrefix) || !strings.HasSuffix(s, ";\n") {
			t.Fatalf("releases.js fora do formato: %q", s)
		}
		var idx update.Index
		if err := json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(s, releasesJSPrefix), ";\n")), &idx); err != nil {
			t.Fatalf("releases.js não carrega um objeto JSON: %v", err)
		}
		return idx
	}
	if got, want := lerJS(), lerIndice(t, web); !reflect.DeepEqual(got, want) {
		t.Errorf("releases.js diverge do index.json:\n%+v\n%+v", got, want)
	}

	// Regenerate recria o arquivo sumido com o mesmo conteúdo.
	if err := os.Remove(filepath.Join(web, "releases", "releases.js")); err != nil {
		t.Fatal(err)
	}
	if err := Regenerate(web); err != nil {
		t.Fatal(err)
	}
	if got, want := lerJS(), lerIndice(t, web); !reflect.DeepEqual(got, want) {
		t.Errorf("releases.js regenerado diverge do index.json")
	}
}
