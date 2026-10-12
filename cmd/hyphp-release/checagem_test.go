package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"hyphp/internal/pkgmgr"
)

// mundo é o site do catálogo e o Apache Lounge num servidor HTTPS falso: o
// catálogo só aceita URLs https.
type mundo struct {
	t     *testing.T
	srv   *httptest.Server
	rotas map[string]rota
	pub   ed25519.PublicKey
	priv  ed25519.PrivateKey
}

type rota struct {
	status int
	tipo   string
	corpo  []byte
}

const apacheAtual = "/download/VS18/binaries/httpd-2.4.69-261002-Win64-VS18.zip"

func novoMundo(t *testing.T) *mundo {
	t.Helper()
	w := &mundo{t: t, rotas: map[string]rota{}}
	var err error
	if w.pub, w.priv, err = ed25519.GenerateKey(rand.Reader); err != nil {
		t.Fatal(err)
	}
	w.srv = httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ro, ok := w.rotas[r.URL.Path]
		if !ok {
			http.NotFound(rw, r)
			return
		}
		rw.Header().Set("Content-Type", ro.tipo)
		rw.WriteHeader(ro.status)
		_, _ = rw.Write(ro.corpo)
	}))
	t.Cleanup(w.srv.Close)
	return w
}

func (w *mundo) servir(caminho string, status int, tipo string, corpo []byte) {
	w.rotas[caminho] = rota{status, tipo, corpo}
}

func (w *mundo) arquivo(caminho string) {
	w.servir(caminho, http.StatusOK, "application/zip", []byte("PK"))
}

// catalogo monta um catálogo com o Apache atual e os pacotes dados.
func (w *mundo) catalogo(extras ...pkgmgr.Package) pkgmgr.Catalog {
	apache := pkgmgr.Package{
		ID: "apache-2.4.69-vs18-x64", Kind: "apache", Version: "2.4.69",
		URL: w.srv.URL + apacheAtual, SHA256: strings.Repeat("a", 64),
		Notes: "Apache Lounge build 261002. SHA-256 de " + w.srv.URL + apacheAtual + ".txt -> instalado em bin/apache/apache-2.4.69-vs18-x64/.",
	}
	return pkgmgr.Catalog{Schema: 1, Serial: 2026101100, Packages: append([]pkgmgr.Package{apache}, extras...)}
}

// publicar serve o catálogo assinado com priv, como o catalogo.yml.
func (w *mundo) publicar(c pkgmgr.Catalog, priv ed25519.PrivateKey) {
	raw, err := json.Marshal(c)
	if err != nil {
		w.t.Fatal(err)
	}
	w.servir("/catalogo/catalog.json", http.StatusOK, "application/json", raw)
	w.servir("/catalogo/catalog.json.sig", http.StatusOK, "text/plain", []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, raw))+"\n"))
}

// pagina serve a página de download do Lounge com os zips dados (cada um
// com o .txt ao lado, como a real).
func (w *mundo) pagina(zips ...string) {
	var b strings.Builder
	b.WriteString("<html><body>")
	for _, z := range zips {
		fmt.Fprintf(&b, `<a href="%s">zip</a> <a href="%s.txt">txt</a> <a href="%s.asc">asc</a>`, z, z, z)
	}
	w.servir("/download/", http.StatusOK, "text/html", []byte(b.String()))
}

// txtLounge imita o .txt do Lounge: SHA1, depois SHA256, em maiúsculas.
func txtLounge(arquivo string, zip []byte) []byte {
	sum := sha256.Sum256(zip)
	return []byte("\ufeffChecksums created with GPGHash\r\n\r\nSHA1-Checksum for: " + arquivo + ":\r\nD7B557581787727CA222C6A5FF0E9E6FA6029893\r\n\r\nSHA256-Checksum for: " + arquivo + ":\r\n" + strings.ToUpper(hex.EncodeToString(sum[:])) + "\r\n")
}

func (w *mundo) checagem(reserva pkgmgr.Catalog) (Checagem, *strings.Builder) {
	log := &strings.Builder{}
	return Checagem{
		Client: w.srv.Client(), CatalogURL: w.srv.URL + "/catalogo/catalog.json", Key: w.pub,
		Reserva: reserva, LoungeURL: w.srv.URL + "/download/", TmpDir: w.t.TempDir(), Log: log,
	}, log
}

func titulos(achados []Achado) []string {
	var out []string
	for _, a := range achados {
		out = append(out, a.Titulo)
	}
	return out
}

func achado(achados []Achado, titulo string) (Achado, bool) {
	i := slices.IndexFunc(achados, func(a Achado) bool { return a.Titulo == titulo })
	if i < 0 {
		return Achado{}, false
	}
	return achados[i], true
}

// Sem build nova e com todas as fontes no ar, nada vira issue.
func TestChecagemSemNovidade(t *testing.T) {
	w := novoMundo(t)
	w.publicar(w.catalogo(), w.priv)
	w.pagina(apacheAtual)
	w.arquivo(apacheAtual)
	ch, _ := w.checagem(pkgmgr.Catalog{})
	achados, err := ch.Rodar(context.Background())
	if err != nil || len(achados) != 0 {
		t.Fatalf("achados %q, err %v", titulos(achados), err)
	}
}

// O Lounge publicou a build 261102 e apagou a 261002: a issue traz o
// SHA-256 conferido com o .txt e a entrada do catalog.json pronta, com a nota
// atualizada. O zip baixado não fica no disco.
func TestChecagemBuildNovaDoApache(t *testing.T) {
	w := novoMundo(t)
	w.publicar(w.catalogo(), w.priv)
	nova := "/download/VS18/binaries/httpd-2.4.70-261102-Win64-VS18.zip"
	zip := []byte("zip do apache 2.4.70")
	sum := sha256.Sum256(zip)
	w.pagina(nova)
	w.servir(nova, http.StatusOK, "application/zip", zip)
	w.servir(nova+".txt", http.StatusOK, "text/plain", txtLounge("httpd-2.4.70-261102-Win64-VS18.zip", zip))
	ch, _ := w.checagem(pkgmgr.Catalog{})
	achados, err := ch.Rodar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a, ok := achado(achados, "catálogo: Apache 2.4.70 build 261102 no Apache Lounge")
	if !ok || !a.Unico || len(achados) != 1 {
		t.Fatalf("quer só a issue única da build nova (o link antigo apagado não vira outra): %q", titulos(achados))
	}
	for _, quer := range []string{
		hex.EncodeToString(sum[:]),
		`"id": "apache-2.4.70-vs18-x64"`,
		`"url": "` + w.srv.URL + nova + `"`,
		"Apache Lounge build 261102",
		"bin/apache/apache-2.4.70-vs18-x64/",
		"->",
		"não responde mais",
	} {
		if !strings.Contains(a.Corpo, quer) {
			t.Errorf("corpo sem %q:\n%s", quer, a.Corpo)
		}
	}
	if entradas, _ := os.ReadDir(ch.TmpDir); len(entradas) != 0 {
		t.Error("o zip baixado ficou no disco")
	}
}

// SHA-256 do zip que não bate com o .txt do Lounge: o job falha e a build
// não vira issue.
func TestChecagemShaDoLoungeNaoConfere(t *testing.T) {
	w := novoMundo(t)
	w.publicar(w.catalogo(), w.priv)
	nova := "/download/VS18/binaries/httpd-2.4.70-261102-Win64-VS18.zip"
	w.pagina(nova)
	w.servir(nova, http.StatusOK, "application/zip", []byte("zip trocado"))
	w.servir(nova+".txt", http.StatusOK, "text/plain", txtLounge("httpd-2.4.70-261102-Win64-VS18.zip", []byte("zip original")))
	w.arquivo(apacheAtual)
	ch, _ := w.checagem(pkgmgr.Catalog{})
	achados, err := ch.Rodar(context.Background())
	if err == nil || !strings.Contains(err.Error(), "não confere") {
		t.Fatalf("err = %v, quer o SHA-256 que não confere", err)
	}
	if _, ok := achado(achados, "catálogo: Apache 2.4.70 build 261102 no Apache Lounge"); ok {
		t.Error("build com SHA-256 errado virou issue")
	}
}

// Página que responde sem nenhuma build VS18 x64: o Lounge mudou o layout
// ou o compilador.
func TestChecagemPaginaDoLoungeMudou(t *testing.T) {
	w := novoMundo(t)
	w.publicar(w.catalogo(), w.priv)
	w.pagina("/download/VS19/binaries/httpd-2.4.70-261102-Win64-VS19.zip")
	w.arquivo(apacheAtual)
	ch, _ := w.checagem(pkgmgr.Catalog{})
	achados, err := ch.Rodar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := achado(achados, "catálogo: página do Apache Lounge mudou"); !ok || len(achados) != 1 {
		t.Fatalf("achados = %q", titulos(achados))
	}
}

// Lounge fora do ar não é problema do catálogo: o job falha, e o que as
// outras checagens acharam vira issue mesmo assim.
func TestChecagemLoungeForaDoAr(t *testing.T) {
	w := novoMundo(t)
	quebrado := pkgmgr.Package{ID: "mailpit-1.31.1", Kind: "mailpit", Version: "1.31.1", URL: w.srv.URL + "/mailpit.zip", SHA256: strings.Repeat("c", 64)}
	w.publicar(w.catalogo(quebrado), w.priv)
	w.servir("/download/", http.StatusServiceUnavailable, "text/html", []byte("fora do ar"))
	w.arquivo(apacheAtual)
	ch, _ := w.checagem(pkgmgr.Catalog{})
	achados, err := ch.Rodar(context.Background())
	if err == nil {
		t.Fatal("Lounge fora do ar não fez o job falhar")
	}
	if _, ok := achado(achados, "catálogo: mailpit-1.31.1 sem fonte disponível"); !ok {
		t.Errorf("achados = %q, quer o mailpit sem fonte", titulos(achados))
	}
}

// Pacote sem nenhuma fonte boa vira issue com cada tentativa; principal
// caído com mirror bom, ou mirror caído com principal bom, só vai para o
// log, porque o download já cai para a fonte que responde.
func TestChecagemFontes(t *testing.T) {
	w := novoMundo(t)
	sha := strings.Repeat("b", 64)
	comMirror := pkgmgr.Package{ID: "php-8.3.35-nts-vs16-x64", Kind: "php", Version: "8.3.35",
		URL: w.srv.URL + "/releases/php.zip", Mirrors: []string{w.srv.URL + "/archives/php.zip"}, SHA256: sha}
	mirrorCaido := pkgmgr.Package{ID: "mailpit-1.31.1", Kind: "mailpit", Version: "1.31.1",
		URL: w.srv.URL + "/mailpit.zip", Mirrors: []string{w.srv.URL + "/espelho/mailpit.zip"}, SHA256: sha}
	semFonte := pkgmgr.Package{ID: "nginx-1.30.5", Kind: "nginx", Version: "1.30.5",
		URL: w.srv.URL + "/nginx.zip", Mirrors: []string{w.srv.URL + "/espelho/nginx.zip"}, SHA256: sha}
	w.publicar(w.catalogo(comMirror, mirrorCaido, semFonte), w.priv)
	w.pagina(apacheAtual)
	w.arquivo(apacheAtual)
	w.arquivo("/archives/php.zip")
	w.arquivo("/mailpit.zip")
	w.servir("/nginx.zip", http.StatusOK, "text/html", []byte("<html>Oops</html>"))
	ch, log := w.checagem(pkgmgr.Catalog{})
	achados, err := ch.Rodar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a, ok := achado(achados, "catálogo: nginx-1.30.5 sem fonte disponível")
	if !ok || len(achados) != 1 {
		t.Fatalf("achados = %q", titulos(achados))
	}
	for _, quer := range []string{"/nginx.zip", "text/html", "/espelho/nginx.zip", "404"} {
		if !strings.Contains(a.Corpo, quer) {
			t.Errorf("corpo sem %q:\n%s", quer, a.Corpo)
		}
	}
	if !strings.Contains(log.String(), "php-8.3.35-nts-vs16-x64") || !strings.Contains(log.String(), "/archives/php.zip") {
		t.Errorf("o principal caído com mirror bom não foi para o log:\n%s", log)
	}
	if !strings.Contains(log.String(), "mailpit-1.31.1: o mirror "+w.srv.URL+"/espelho/mailpit.zip falhou") {
		t.Errorf("o mirror caído com principal bom não foi para o log:\n%s", log)
	}
}

// Catálogo publicado com assinatura de outra chave: issue, e o resto da
// checagem usa o catálogo de reserva (o embutido no binário).
func TestChecagemCatalogoPublicadoInvalido(t *testing.T) {
	w := novoMundo(t)
	_, outra, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	w.publicar(w.catalogo(), outra)
	reserva := w.catalogo(pkgmgr.Package{ID: "mailpit-1.31.1", Kind: "mailpit", Version: "1.31.1",
		URL: w.srv.URL + "/mailpit.zip", SHA256: strings.Repeat("c", 64)})
	w.pagina(apacheAtual)
	w.arquivo(apacheAtual)
	ch, _ := w.checagem(reserva)
	achados, err := ch.Rodar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, quer := range []string{"catálogo: catálogo publicado inacessível ou inválido", "catálogo: mailpit-1.31.1 sem fonte disponível"} {
		if _, ok := achado(achados, quer); !ok {
			t.Errorf("achados = %q, quer %q", titulos(achados), quer)
		}
	}
}

// ghIssues imita o gh das issues: lista as existentes do rótulo e registra
// as criadas.
type ghIssues struct {
	abertas, fechadas []string
	labels            int
	criadas           []string
}

func valorDe(args []string, flag string) string {
	for i := range len(args) - 1 {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func (g *ghIssues) run(args ...string) ([]byte, error) {
	switch {
	case len(args) > 1 && args[0] == "label" && args[1] == "create":
		g.labels++
		return nil, nil
	case len(args) > 1 && args[0] == "issue" && args[1] == "list":
		lista := slices.Clone(g.abertas)
		if valorDe(args, "--state") == "all" {
			lista = append(lista, g.fechadas...)
		}
		var out []map[string]string
		for _, titulo := range lista {
			out = append(out, map[string]string{"title": titulo})
		}
		return json.Marshal(out)
	case len(args) > 1 && args[0] == "issue" && args[1] == "create":
		if valorDe(args, "--label") != rotuloChecagem {
			return nil, fmt.Errorf("issue sem o rótulo %s: %q", rotuloChecagem, args)
		}
		g.criadas = append(g.criadas, valorDe(args, "--title"))
		return nil, nil
	}
	return nil, fmt.Errorf("gh falso: comando inesperado %q", args)
}

// Uma issue por problema: a build do Apache não volta depois de fechada; os
// demais problemas voltam se reaparecerem com a issue fechada.
func TestAbrirIssuesSemRepetir(t *testing.T) {
	g := &ghIssues{
		abertas:  []string{"catálogo: nginx-1.30.5 sem fonte disponível"},
		fechadas: []string{"catálogo: Apache 2.4.70 build 261102 no Apache Lounge", "catálogo: página do Apache Lounge mudou"},
	}
	achados := []Achado{
		{Titulo: "catálogo: Apache 2.4.70 build 261102 no Apache Lounge", Unico: true},
		{Titulo: "catálogo: nginx-1.30.5 sem fonte disponível"},
		{Titulo: "catálogo: página do Apache Lounge mudou"},
		{Titulo: "catálogo: mailpit-1.31.1 sem fonte disponível"},
	}
	if err := abrirIssues(g.run, "dono/repo", achados, io.Discard); err != nil {
		t.Fatal(err)
	}
	quer := []string{"catálogo: página do Apache Lounge mudou", "catálogo: mailpit-1.31.1 sem fonte disponível"}
	if !slices.Equal(g.criadas, quer) {
		t.Errorf("criadas = %q, quer %q", g.criadas, quer)
	}
	if g.labels != 1 {
		t.Errorf("rótulo criado %d vezes, quer 1", g.labels)
	}
}

// Sem achado, o gh não roda: a checagem passa todo dia sem problema e não
// pode mexer no repositório (nem criar o rótulo).
func TestAbrirIssuesSemAchadoNaoChamaOGh(t *testing.T) {
	gh := func(args ...string) ([]byte, error) {
		t.Errorf("gh chamado sem achado: %q", args)
		return nil, nil
	}
	if err := abrirIssues(gh, "dono/repo", nil, io.Discard); err != nil {
		t.Fatal(err)
	}
}
