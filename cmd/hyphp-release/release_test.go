package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"hyphp/internal/update"
)

const testRepo = "dono/hyphp"

// ambiente cria o checkout vazio do site, os quatro arquivos que os jobs de
// build entregam e um par de chaves.
func ambiente(t *testing.T) (web string, f Files, pub ed25519.PublicKey, priv ed25519.PrivateKey) {
	t.Helper()
	web = t.TempDir()
	dir := t.TempDir()
	f = Files{
		Installer: filepath.Join(dir, "hyphp-amd64-installer.exe"),
		DMG:       filepath.Join(dir, "HyPHP.dmg"),
		Install:   filepath.Join(dir, "install.sh"),
		Uninstall: filepath.Join(dir, "uninstall.sh"),
	}
	for p, conteudo := range map[string]string{
		f.Installer: "instalador de teste",
		f.DMG:       "dmg de teste",
		f.Install:   "#!/bin/sh\n# install\n",
		f.Uninstall: "#!/bin/sh\n# uninstall\n",
	} {
		if err := os.WriteFile(p, []byte(conteudo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return web, f, pub, priv
}

// ghFalso faz o papel do GitHub CLI. Guarda o conteúdo de cada asset na hora
// do create (o Publish apaga a pasta temporária ao sair), responde o view com
// o digest do que recebeu, como o GitHub, e registra a ordem das chamadas.
type ghFalso struct {
	chamadas  [][]string
	assets    map[string][]byte // nome do asset → conteúdo recebido
	notas     []byte            // conteúdo do --notes-file
	ultima    string            // tagName da última release publicada
	corromper string            // asset cujo digest o view devolve errado
	falha     error             // devolvida pelo create
	lista     []releaseListada  // resposta do release list
}

type releaseListada struct {
	TagName string `json:"tagName"`
	IsDraft bool   `json:"isDraft"`
}

func novoGH() *ghFalso {
	return &ghFalso{assets: map[string][]byte{}, ultima: "v0.9.0"}
}

func (g *ghFalso) run(args ...string) ([]byte, error) {
	g.chamadas = append(g.chamadas, args)
	if len(args) < 2 || args[0] != "release" {
		return nil, fmt.Errorf("gh falso: comando inesperado %q", args)
	}
	switch args[1] {
	case "list":
		return json.Marshal(append([]releaseListada{}, g.lista...))
	case "create":
		if g.falha != nil {
			return nil, g.falha
		}
		i := slices.Index(args, "--notes-file")
		if i < 0 || i+1 >= len(args) {
			return nil, errors.New("gh falso: create sem --notes-file")
		}
		g.notas, _ = os.ReadFile(args[i+1])
		for _, a := range args[i+2:] {
			b, err := os.ReadFile(a)
			if err != nil {
				return nil, err
			}
			g.assets[filepath.Base(a)] = b
		}
		return nil, nil
	case "view":
		if args[len(args)-1] == "tagName" {
			return json.Marshal(map[string]string{"tagName": g.ultima})
		}
		type asset struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		}
		var v struct {
			Assets []asset `json:"assets"`
		}
		for nome, b := range g.assets {
			sum := sha256.Sum256(b)
			d := "sha256:" + hex.EncodeToString(sum[:])
			if nome == g.corromper {
				d = "sha256:" + strings.Repeat("0", 64)
			}
			v.Assets = append(v.Assets, asset{nome, d})
		}
		return json.Marshal(v)
	}
	return nil, nil
}

func publicar(t *testing.T, web string, f Files, v string, priv ed25519.PrivateKey, gh *ghFalso) error {
	t.Helper()
	_, err := Publish(Options{
		WebDir: web, Repo: testRepo, Version: v, Date: "2026-10-11",
		Notes: Notes{PT: []string{"nota " + v}, EN: []string{"note " + v}},
		Files: f, Key: priv, GH: gh.run,
	})
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

// escreverVersoes grava os arquivos de versão que checkVersions lê.
func escreverVersoes(t *testing.T, root, cfg, info, nsh, plist string) {
	t.Helper()
	for _, dir := range []string{filepath.Join(root, "build", "windows", "nsis"), filepath.Join(root, "build", "darwin")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for p, conteudo := range map[string]string{
		filepath.Join(root, "build", "config.yml"):                         "version: '3'\ninfo:\n  productName: \"HyPHP\"\n  version: \"" + cfg + "\"\n",
		filepath.Join(root, "build", "windows", "info.json"):               `{"fixed":{"file_version":"` + info + `"},"info":{"0000":{"ProductVersion":"` + info + `"}}}`,
		filepath.Join(root, "build", "windows", "nsis", "wails_tools.nsh"): "!ifndef INFO_PRODUCTVERSION\n    !define INFO_PRODUCTVERSION \"" + nsh + "\"\n!endif\n",
		filepath.Join(root, "build", "darwin", "Info.plist"):               "<plist version=\"1.0\">\n\t<dict>\n\t\t<key>CFBundleShortVersionString</key>\n\t\t<string>" + plist + "</string>\n\t\t<key>CFBundleVersion</key>\n\t\t<string>" + plist + "</string>\n\t</dict>\n</plist>\n",
	} {
		if err := os.WriteFile(p, []byte(conteudo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func escreverNotas(t *testing.T, repo, versao, conteudo string) {
	t.Helper()
	p := notesPath(repo, versao)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}
}

// O que vai para a release é o que o app instalado lê: precisa passar pelo
// mesmo caminho que ele usa, Verify sobre os bytes enviados e ParseLatest, e
// cada path do manifesto tem de ser o nome de um asset enviado, com o
// tamanho do arquivo que o job de build entregou.
func TestPublishCriaReleaseQueOAppAceita(t *testing.T) {
	web, f, pub, priv := ambiente(t)
	gh := novoGH()
	if err := publicar(t, web, f, "1.0.0", priv, gh); err != nil {
		t.Fatal(err)
	}
	if got := string(gh.notas); got != "## English\n\n- note 1.0.0\n\n## Português\n\n- nota 1.0.0\n" {
		t.Errorf("notas da release = %q", got)
	}
	body, sig := gh.assets["latest.json"], gh.assets["latest.json.sig"]
	if err := update.Verify(pub, body, sig); err != nil {
		t.Fatalf("app recusaria a assinatura: %v", err)
	}
	l, err := update.ParseLatest(body)
	if err != nil {
		t.Fatalf("app recusaria o manifesto: %v", err)
	}
	if !slices.Equal(l.Notes, []string{"nota 1.0.0"}) || !slices.Equal(l.NotesEN, []string{"note 1.0.0"}) {
		t.Errorf("notas do manifesto: pt %q, en %q", l.Notes, l.NotesEN)
	}
	for _, c := range []struct {
		a              *update.Artifact
		nome, conteudo string
	}{
		{l.WindowsAMD64, "hyphp-1.0.0-windows-amd64-setup.exe", "instalador de teste"},
		{l.DarwinARM64, "hyphp-1.0.0-darwin-arm64.dmg", "dmg de teste"},
	} {
		if c.a == nil || c.a.Path != c.nome {
			t.Fatalf("artefato %+v, quer path %s", c.a, c.nome)
		}
		if b := gh.assets[c.nome]; string(b) != c.conteudo || int64(len(b)) != c.a.Size {
			t.Errorf("asset %s não é o arquivo do job de build", c.nome)
		}
	}
	// Os scripts vão com o nome fixo: é o que o curl de latest/download/ busca.
	for _, nome := range []string{"install.sh", "uninstall.sh"} {
		if _, ok := gh.assets[nome]; !ok {
			t.Errorf("%s não foi para a release", nome)
		}
	}
	if len(gh.assets) != 6 {
		t.Errorf("%d assets enviados, quer 6", len(gh.assets))
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

// A release só vira pública e Latest depois que os digests batem: publicar
// antes deixaria os apps baixarem um manifesto ainda não conferido.
func TestPublishOrdemDraftConferenciaPublicacao(t *testing.T) {
	web, f, _, priv := ambiente(t)
	gh := novoGH()
	if err := publicar(t, web, f, "1.0.0", priv, gh); err != nil {
		t.Fatal(err)
	}
	var ops []string
	for _, c := range gh.chamadas {
		ops = append(ops, c[0]+" "+c[1])
	}
	if want := []string{"release list", "release create", "release view", "release edit"}; !slices.Equal(ops, want) {
		t.Fatalf("ordem %q, quer %q", ops, want)
	}
	create := gh.chamadas[1]
	if want := []string{"release", "create", "v1.0.0", "--repo", testRepo, "--draft", "--verify-tag", "--title", "HyPHP 1.0.0"}; !slices.Equal(create[:len(want)], want) {
		t.Errorf("create = %q", create)
	}
	if slices.Contains(create, "--latest") {
		t.Error("o create já marca Latest, antes da conferência")
	}
	if want := []string{"release", "edit", "v1.0.0", "--repo", testRepo, "--draft=false", "--latest"}; !slices.Equal(gh.chamadas[3], want) {
		t.Errorf("edit = %q", gh.chamadas[3])
	}
}

// Uma publicação que parou depois do create deixou um draft com a tag: criar
// outro deixaria dois drafts com o mesmo tag_name. A reexecução é recusada,
// dizendo como apagar o draft, e nada é criado.
func TestPublishRecusaDraftDePublicacaoAnterior(t *testing.T) {
	web, f, _, priv := ambiente(t)
	gh := novoGH()
	gh.lista = []releaseListada{{TagName: "v0.9.0"}, {TagName: "v1.0.0", IsDraft: true}}
	err := publicar(t, web, f, "1.0.0", priv, gh)
	if err == nil || !strings.Contains(err.Error(), "gh release delete v1.0.0 --repo "+testRepo+" --yes") {
		t.Fatalf("erro = %v, quer a instrução de apagar o draft", err)
	}
	if len(gh.chamadas) != 1 {
		t.Errorf("chamadas %q; a recusa vem antes do create", gh.chamadas)
	}
}

// A tag já tem release publicada (o index.json do site ficou para trás): ela é
// imutável, então a publicação é recusada sem create.
func TestPublishRecusaReleaseJaPublicada(t *testing.T) {
	web, f, _, priv := ambiente(t)
	gh := novoGH()
	gh.lista = []releaseListada{{TagName: "v1.0.0"}}
	err := publicar(t, web, f, "1.0.0", priv, gh)
	if err == nil || !strings.Contains(err.Error(), "publicada") {
		t.Fatalf("erro = %v, quer recusa da release publicada", err)
	}
	if len(gh.chamadas) != 1 {
		t.Errorf("chamadas %q; a recusa vem antes do create", gh.chamadas)
	}
	if _, err := os.Stat(filepath.Join(web, "releases")); !os.IsNotExist(err) {
		t.Error("o site mudou com a publicação recusada")
	}
}

// O GitHub guardou bytes diferentes dos enviados (upload truncado, asset
// trocado): a release fica em draft, invisível, e o site não anuncia nada.
func TestPublishDigestDiferenteFicaEmDraft(t *testing.T) {
	for _, asset := range []string{"hyphp-1.0.0-darwin-arm64.dmg", "latest.json.sig", "install.sh"} {
		t.Run(asset, func(t *testing.T) {
			web, f, _, priv := ambiente(t)
			gh := novoGH()
			gh.corromper = asset
			err := publicar(t, web, f, "1.0.0", priv, gh)
			if err == nil || !strings.Contains(err.Error(), asset) {
				t.Fatalf("erro = %v, quer citar %s", err, asset)
			}
			for _, c := range gh.chamadas {
				if c[1] == "edit" {
					t.Fatal("a release saiu do draft com digest divergente")
				}
			}
			if _, err := os.Stat(filepath.Join(web, "releases")); !os.IsNotExist(err) {
				t.Error("o site ganhou arquivos de uma release que ficou em draft")
			}
		})
	}
}

func TestPublishOrdenaPorVersaoNumerica(t *testing.T) {
	web, f, _, priv := ambiente(t)
	gh := novoGH()
	for _, v := range []string{"1.0.9", "1.0.10"} {
		if err := publicar(t, web, f, v, priv, gh); err != nil {
			t.Fatal(err)
		}
	}
	idx := lerIndice(t, web)
	if len(idx.Releases) != 2 || idx.Releases[0].Version != "1.0.10" || idx.Releases[1].Version != "1.0.9" {
		t.Errorf("ordem errada: %+v", idx.Releases)
	}
}

// Release publicada é imutável no GitHub: a mesma versão, ou uma menor que a
// última do site, é recusada antes de chamar o gh.
func TestPublishRecusaRepublicarOuRegredir(t *testing.T) {
	web, f, _, priv := ambiente(t)
	gh := novoGH()
	if err := publicar(t, web, f, "1.1.0", priv, gh); err != nil {
		t.Fatal(err)
	}
	antes, _ := os.ReadFile(filepath.Join(web, "releases", "index.json"))

	if err := publicar(t, web, f, "1.1.0", priv, gh); err == nil {
		t.Error("republicou a mesma versão")
	}
	if err := publicar(t, web, f, "1.0.5", priv, gh); err == nil {
		t.Error("publicou versão menor que a última")
	}
	if len(gh.chamadas) != 4 {
		t.Errorf("gh chamado %d vezes; as recusas não podiam chegar nele", len(gh.chamadas))
	}
	if depois, _ := os.ReadFile(filepath.Join(web, "releases", "index.json")); string(antes) != string(depois) {
		t.Error("index.json mudou depois de publicações recusadas")
	}
}

// gh falhou (sem login, tag ausente, rede): o site não pode anunciar uma
// versão que não existe no GitHub.
func TestPublishGHFalhouNaoMexeNoSite(t *testing.T) {
	web, f, _, priv := ambiente(t)
	gh := novoGH()
	gh.falha = errors.New("tag v1.0.0 não existe")
	err := publicar(t, web, f, "1.0.0", priv, gh)
	if err == nil || !strings.Contains(err.Error(), "tag v1.0.0") {
		t.Fatalf("erro do gh não voltou: %v", err)
	}
	if _, err := os.Stat(filepath.Join(web, "releases")); !os.IsNotExist(err) {
		t.Error("o site ganhou arquivos de uma release que falhou")
	}
}

// O site mostra as notas do idioma escolhido e a release não aceita correção:
// publicar com um idioma faltando, ou com uma nota sem tradução, é recusado
// antes do gh.
func TestPublishExigeNotasNosDoisIdiomas(t *testing.T) {
	casos := []struct {
		nome   string
		pt, en []string
	}{
		{"sem inglês", []string{"Corrige X"}, nil},
		{"sem português", nil, []string{"Fixes X"}},
		{"nota sem tradução", []string{"Corrige X", "Adiciona Y"}, []string{"Fixes X"}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			web, f, _, priv := ambiente(t)
			gh := novoGH()
			_, err := Publish(Options{WebDir: web, Repo: testRepo, Version: "1.0.0", Date: "2026-10-11", Notes: Notes{PT: c.pt, EN: c.en}, Files: f, Key: priv, GH: gh.run})
			if err == nil {
				t.Fatal("publicou sem as notas nos dois idiomas")
			}
			if len(gh.chamadas) != 0 {
				t.Error("a recusa chegou no gh")
			}
		})
	}
}

// As notas moram num arquivo revisado no PR da versão. Ausente, vazio, sem
// tradução ou com chave trocada ("pt-BR" no lugar de "pt" deixaria o
// português de fora sem aviso): o job verificar para antes de compilar.
func TestNotasDaVersao(t *testing.T) {
	repo := t.TempDir()
	if _, err := readNotes(repo, "1.0.0"); err == nil || !strings.Contains(err.Error(), "ausente") {
		t.Errorf("arquivo ausente: erro = %v", err)
	}
	for _, c := range []struct{ nome, conteudo string }{
		{"vazias", `{"pt":[],"en":[]}`},
		{"sem inglês", `{"pt":["Corrige X"]}`},
		{"nota sem tradução", `{"pt":["Corrige X","Adiciona Y"],"en":["Fixes X"]}`},
		{"chave desconhecida", `{"pt-BR":["Corrige X"],"pt":["Corrige X"],"en":["Fixes X"]}`},
		{"JSON quebrado", `{"pt":["Corrige X"],`},
	} {
		t.Run(c.nome, func(t *testing.T) {
			escreverNotas(t, repo, "1.0.0", c.conteudo)
			if _, err := readNotes(repo, "1.0.0"); err == nil {
				t.Error("notas aceitas")
			}
		})
	}
	escreverNotas(t, repo, "1.0.0", `{"pt":["Corrige X"],"en":["Fixes X"]}`)
	n, err := readNotes(repo, "1.0.0")
	if err != nil || !slices.Equal(n.PT, []string{"Corrige X"}) || !slices.Equal(n.EN, []string{"Fixes X"}) {
		t.Errorf("notas = %+v, %v", n, err)
	}
}

// O job verificar roda antes de compilar: tag que não é a versão compilada,
// versão que não passa da última release ou notas ausentes param tudo ali. O
// ensaio, sem tag, confere só os arquivos de versão e não fala com o GitHub.
func TestVerificar(t *testing.T) {
	repo := t.TempDir()
	escreverVersoes(t, repo, "1.1.0", "1.1.0", "1.1.0", "1.1.0")
	escreverNotas(t, repo, "1.1.0", `{"pt":["Corrige X"],"en":["Fixes X"]}`)
	gh := novoGH()
	gh.ultima = "v1.0.0"
	if err := verify(repo, "1.1.0", "v1.1.0", testRepo, gh.run); err != nil {
		t.Fatalf("versão certa recusada: %v", err)
	}
	if err := verify(repo, "1.1.0", "v1.1.1", testRepo, gh.run); err == nil {
		t.Error("tag diferente da versão compilada aceita")
	}
	for _, ultima := range []string{"v1.1.0", "v1.2.0"} {
		gh.ultima = ultima
		if err := verify(repo, "1.1.0", "v1.1.0", testRepo, gh.run); err == nil {
			t.Errorf("versão 1.1.0 aceita com a última release em %s", ultima)
		}
	}
	gh.ultima = "v1.0.0"
	if err := os.Remove(notesPath(repo, "1.1.0")); err != nil {
		t.Fatal(err)
	}
	if err := verify(repo, "1.1.0", "v1.1.0", testRepo, gh.run); err == nil {
		t.Error("publicação sem o arquivo de notas aceita")
	}
	semGH := func(...string) ([]byte, error) {
		t.Error("o ensaio consultou o GitHub")
		return nil, errors.New("sem gh")
	}
	if err := verify(repo, "1.1.0", "", testRepo, semGH); err != nil {
		t.Errorf("ensaio recusado: %v", err)
	}
	escreverVersoes(t, repo, "1.1.0", "1.1.0", "1.0.0", "1.1.0")
	if err := verify(repo, "1.1.0", "", testRepo, semGH); err == nil {
		t.Error("ensaio aceitou arquivos de versão divergentes")
	}
}

// A chave vem do segredo do environment. Uma seed que não é a da chave
// pública embutida assinaria manifestos que todo app instalado recusa.
func TestChaveDoAmbiente(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	seed := hex.EncodeToString(priv.Seed())
	// O gh secret set guarda o arquivo como está, com a quebra de linha final.
	if k, err := releaseKey(seed+"\n", pub); err != nil || !k.Equal(priv) {
		t.Fatalf("seed certa recusada: %v", err)
	}
	_, outra, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := releaseKey(hex.EncodeToString(outra.Seed()), pub); err == nil {
		t.Error("seed de outra chave aceita")
	}
	for _, ruim := range []string{"", "zz", seed[:10]} {
		if _, err := releaseKey(ruim, pub); err == nil {
			t.Errorf("seed %q aceita", ruim)
		}
	}
}

func TestVersoesConsistentes(t *testing.T) {
	root := t.TempDir()
	escreverVersoes(t, root, "1.0.0", "1.0.0", "1.0.0", "1.0.0")
	if err := checkVersions(root, "1.0.0"); err != nil {
		t.Errorf("versões iguais recusadas: %v", err)
	}
	escreverVersoes(t, root, "1.0.0", "0.1.0", "1.0.0", "1.0.0")
	if err := checkVersions(root, "1.0.0"); err == nil {
		t.Error("info.json divergente aceito")
	}
	escreverVersoes(t, root, "1.0.1", "1.0.0", "1.0.0", "1.0.0")
	if err := checkVersions(root, "1.0.0"); err == nil {
		t.Error("config.yml divergente aceito")
	}
	// O instalador sai com a versão do .nsh em "Aplicativos instalados".
	escreverVersoes(t, root, "1.0.0", "1.0.0", "0.1.0", "1.0.0")
	if err := checkVersions(root, "1.0.0"); err == nil {
		t.Error("wails_tools.nsh divergente aceito")
	}
	// Info.plist com versão velha: o macOS mostra em "Sobre" e o update compara com ela.
	escreverVersoes(t, root, "1.0.0", "1.0.0", "1.0.0", "0.1.0")
	if err := checkVersions(root, "1.0.0"); err == nil || !strings.Contains(err.Error(), "build/darwin/Info.plist") {
		t.Errorf("Info.plist divergente: erro = %v, quer citar build/darwin/Info.plist", err)
	}
}

// stringVersao monta uma String da StringTable do VS_VERSIONINFO como o NSIS
// grava: wLength, wValueLength (caracteres com o NUL), wType = 1, szKey com
// NUL, preenchimento até 4 bytes, valor com NUL e o alinhamento da próxima.
func stringVersao(chave, valor string) []byte {
	b := append(make([]byte, 6), utf16LE(chave+"\x00")...)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	v := utf16LE(valor + "\x00")
	b = append(b, v...)
	binary.LittleEndian.PutUint16(b[0:], uint16(len(b)))
	binary.LittleEndian.PutUint16(b[2:], uint16(len(v)/2))
	binary.LittleEndian.PutUint16(b[4:], 1)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

// Versão subiu sem reempacotar: o bin/ ainda tem o instalador anterior, e
// publicá-lo numa release imutável põe todo app em loop de update. A
// FileVersion diferente ao lado garante que a chave lida é a ProductVersion.
func TestVersaoDoInstalador(t *testing.T) {
	rsrc := slices.Concat(
		[]byte("cabeçalho de recursos qualquer"),
		stringVersao("FileVersion", "9.9.9"),
		stringVersao("ProductVersion", "3.0.1"),
		stringVersao("ProductName", "HyPHP"),
	)
	if err := checkProductVersion(rsrc, "3.0.1"); err != nil {
		t.Errorf("instalador da versão certa recusado: %v", err)
	}
	err := checkProductVersion(rsrc, "3.0.2")
	if err == nil || !strings.Contains(err.Error(), `"3.0.1"`) || !strings.Contains(err.Error(), "3.0.2") {
		t.Errorf("instalador desatualizado: esperava erro citando as duas versões, veio %v", err)
	}
	if err := checkProductVersion(stringVersao("FileVersion", "3.0.2"), "3.0.2"); err == nil {
		t.Error("recurso sem ProductVersion aceito")
	}
}

// O site é HTML puro e roda aberto do disco, onde fetch() de arquivo local é
// bloqueado; ele lê as versões do releases.js. O conteúdo tem de ser o mesmo
// do index.json, senão o site mostra uma versão e o app baixa outra.
func TestReleasesJSEspelhaOIndice(t *testing.T) {
	web, f, _, priv := ambiente(t)
	gh := novoGH()
	for _, v := range []string{"1.0.0", "1.1.0"} {
		if err := publicar(t, web, f, v, priv, gh); err != nil {
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
