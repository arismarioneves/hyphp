package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"hyphp/internal/update"
)

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

func publicar(t *testing.T, web, inst, v string, priv ed25519.PrivateKey) error {
	t.Helper()
	_, err := Publish(Options{WebDir: web, Installer: inst, Version: v, Date: "2026-09-26", Notes: []string{"nota " + v}, Key: priv})
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

// O que o app instalado vai ler precisa passar exatamente pelo mesmo caminho
// que ele usa: Verify sobre os bytes publicados e depois ParseLatest.
func TestPublishGeraManifestoQueOAppAceita(t *testing.T) {
	web, inst, pub, priv := ambiente(t)
	if err := publicar(t, web, inst, "1.0.0", priv); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(web, "releases")
	body, err := os.ReadFile(filepath.Join(dir, "latest.json"))
	if err != nil {
		t.Fatal(err)
	}
	sig, err := os.ReadFile(filepath.Join(dir, "latest.json.sig"))
	if err != nil {
		t.Fatal(err)
	}
	if err := update.Verify(pub, body, sig); err != nil {
		t.Fatalf("app recusaria a assinatura: %v", err)
	}
	l, err := update.ParseLatest(body)
	if err != nil {
		t.Fatalf("app recusaria o manifesto: %v", err)
	}
	if idx := lerIndice(t, web); !reflect.DeepEqual(l.Release, idx.Releases[0]) {
		t.Errorf("latest.json diverge de index.releases[0]:\n%+v\n%+v", l.Release, idx.Releases[0])
	}
	copia, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(l.WindowsAMD64.Path)))
	if err != nil {
		t.Fatalf("instalador não está no path anunciado: %v", err)
	}
	if int64(len(copia)) != l.WindowsAMD64.Size {
		t.Errorf("size anunciado %d, arquivo tem %d", l.WindowsAMD64.Size, len(copia))
	}
}

func TestPublishOrdenaPorVersaoNumerica(t *testing.T) {
	web, inst, _, priv := ambiente(t)
	for _, v := range []string{"1.0.9", "1.0.10"} {
		if err := publicar(t, web, inst, v, priv); err != nil {
			t.Fatal(err)
		}
	}
	idx := lerIndice(t, web)
	if len(idx.Releases) != 2 || idx.Releases[0].Version != "1.0.10" || idx.Releases[1].Version != "1.0.9" {
		t.Errorf("ordem errada: %+v", idx.Releases)
	}
}

// Arquivo de versão publicada é imutável: o .htaccess manda o CDN guardar o
// .exe por um ano, e republicar serviria o instalador velho com o sha novo.
func TestPublishRecusaRepublicarOuRegredir(t *testing.T) {
	web, inst, _, priv := ambiente(t)
	if err := publicar(t, web, inst, "1.1.0", priv); err != nil {
		t.Fatal(err)
	}
	antes, _ := os.ReadFile(filepath.Join(web, "releases", "latest.json"))

	if err := publicar(t, web, inst, "1.1.0", priv); err == nil {
		t.Error("republicou a mesma versão")
	}
	if err := publicar(t, web, inst, "1.0.5", priv); err == nil {
		t.Error("publicou versão menor que a última")
	}
	depois, _ := os.ReadFile(filepath.Join(web, "releases", "latest.json"))
	if string(antes) != string(depois) {
		t.Error("latest.json mudou depois de publicações recusadas")
	}
	if _, err := os.Stat(filepath.Join(web, "releases", "1.0.5")); !os.IsNotExist(err) {
		t.Error("recusa deixou a pasta 1.0.5 para trás")
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
	for _, v := range []string{"1.0.0", "1.1.0"} {
		if err := publicar(t, web, inst, v, priv); err != nil {
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
