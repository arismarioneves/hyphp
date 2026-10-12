package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"hyphp/internal/pkgmgr"
	"hyphp/internal/runtime"
)

// LoungeURL é a página de download do Apache Lounge. Ela lista só a build
// atual de cada compilador: a anterior some do servidor quando sai a nova.
const LoungeURL = "https://www.apachelounge.com/download/"

const (
	// rotuloChecagem marca as issues abertas pela checagem diária.
	rotuloChecagem = "catalogo"
	// tentativas por endereço: uma oscilação de rede não pode virar issue.
	tentativas = 3
	// agenteChecagem identifica a checagem nos logs dos servidores.
	agenteChecagem = "hyphp-checagem"
	// prazoSonda limita cada GET de 1 byte.
	prazoSonda = 30 * time.Second
)

var (
	// buildLounge acha o zip x64 do compilador do catálogo (VS18) na página
	// do Lounge, que também lista Win32, módulos, .txt e .asc. A aspa final
	// deixa de fora o .zip.txt e o .zip.asc.
	buildLounge = regexp.MustCompile(`/download/VS18/binaries/httpd-(2\.4\.\d+)-(\d+)-Win64-VS18\.zip"`)
	// buildCatalogo lê a build do arquivo do Apache que está no catálogo.
	buildCatalogo = regexp.MustCompile(`httpd-2\.4\.\d+-(\d+)-Win64-VS18\.zip$`)
	// sha256Lounge é a linha do SHA-256 no .txt publicado ao lado de cada
	// zip; o mesmo arquivo traz SHA1, SHA224, SHA384 e SHA512, em maiúsculas.
	sha256Lounge = regexp.MustCompile(`SHA256-Checksum for: [^\r\n]*:\s*([0-9A-Fa-f]{64})`)
)

// Achado é um problema que vira issue. O título é a chave contra issue
// repetida.
type Achado struct {
	Titulo string
	Corpo  string
	// Unico: o título nunca se repete (uma build do Apache), então uma issue
	// com ele, aberta ou fechada, basta. Sem Unico, só não abre outra
	// enquanto houver uma aberta.
	Unico bool
}

// Checagem é o modo -checar-catalogo, rodado todo dia pelo checagem.yml.
type Checagem struct {
	Client     *http.Client
	CatalogURL string            // catálogo publicado (pkgmgr.CatalogURL)
	Key        ed25519.PublicKey // chave pública do catálogo (pkgmgr.CatalogKey)
	// Reserva é o catálogo conferido quando o publicado não se lê: o
	// embutido no binário, que é o do repositório.
	Reserva   pkgmgr.Catalog
	LoungeURL string
	TmpDir    string
	Pausa     time.Duration // entre tentativas; 0 nos testes
	Log       io.Writer
}

// buildApache é uma build do Apache na página do Lounge.
type buildApache struct {
	versao, build, caminho string
}

// Rodar confere o catálogo publicado, procura build nova do Apache e testa
// as fontes de cada pacote. O erro é da parte do Apache (Lounge fora do ar,
// SHA-256 que não confere) e faz o job falhar; os achados valem mesmo com
// ele, para o resto virar issue.
func (c Checagem) Rodar(ctx context.Context) ([]Achado, error) {
	var achados []Achado
	cat, err := c.catalogoPublicado(ctx)
	if err != nil {
		achados = append(achados, Achado{
			Titulo: "catálogo: catálogo publicado inacessível ou inválido",
			Corpo: fmt.Sprintf("A checagem diária não conseguiu ler o catálogo publicado em %s, ou a assinatura não confere com a chave do app:\n\n```\n%v\n```\n\n"+
				"Os apps seguem com o último catálogo que aceitaram. O resto desta checagem usou o catálogo do repositório.", c.CatalogURL, err),
		})
		cat = c.Reserva
	}
	apache, errApache := c.apache(ctx, cat)
	achados = append(achados, apache...)
	// Com build nova, o Lounge já apagou o zip do Apache do catálogo: a issue
	// da build nova cobre isso, e a de "sem fonte" seria repetida.
	pular := ""
	if slices.ContainsFunc(apache, func(a Achado) bool { return a.Unico }) {
		pular = cat.ByKind(runtime.Apache)[0].ID
	}
	achados = append(achados, c.fontes(ctx, cat, pular)...)
	return achados, errApache
}

// catalogoPublicado lê e confere o catálogo como o app faz. Assinatura que
// não confere não é oscilação de rede e não é tentada de novo.
func (c Checagem) catalogoPublicado(ctx context.Context) (pkgmgr.Catalog, error) {
	body, err := c.get(ctx, c.CatalogURL, 1<<20)
	if err != nil {
		return pkgmgr.Catalog{}, err
	}
	sig, err := c.get(ctx, c.CatalogURL+".sig", 4<<10)
	if err != nil {
		return pkgmgr.Catalog{}, err
	}
	if err := pkgmgr.VerifySignature(c.Key, body, sig); err != nil {
		return pkgmgr.Catalog{}, err
	}
	return pkgmgr.ParseCatalog(body)
}

// apache compara a build do Apache no catálogo com a que o Lounge publica.
// Build diferente: baixa o zip novo, confere o SHA-256 com o .txt do Lounge
// e devolve a issue com a entrada do catalog.json pronta.
func (c Checagem) apache(ctx context.Context, cat pkgmgr.Catalog) ([]Achado, error) {
	pkgs := cat.ByKind(runtime.Apache)
	if len(pkgs) != 1 {
		return nil, fmt.Errorf("o catálogo tem %d pacotes do Apache; a checagem compara um só", len(pkgs))
	}
	atual := pkgs[0]
	pagina, err := c.get(ctx, c.LoungeURL, 2<<20)
	if err != nil {
		return nil, fmt.Errorf("página do Apache Lounge: %w", err)
	}
	b, ok := ultimaBuild(pagina)
	if !ok {
		return []Achado{{
			Titulo: "catálogo: página do Apache Lounge mudou",
			Corpo: fmt.Sprintf("A página %s respondeu, mas sem nenhum link `httpd-2.4.x-NNNNNN-Win64-VS18.zip`. "+
				"O Lounge pode ter mudado o layout ou o compilador. Confira a página e a entrada do Apache em `internal/pkgmgr/catalog.json`.", c.LoungeURL),
		}}, nil
	}
	base, err := url.Parse(c.LoungeURL)
	if err != nil {
		return nil, err
	}
	ref, err := url.Parse(b.caminho)
	if err != nil {
		return nil, err
	}
	zipURL := base.ResolveReference(ref).String()
	if path.Base(zipURL) == path.Base(atual.URL) {
		return nil, nil
	}

	dst := filepath.Join(c.TmpDir, path.Base(zipURL))
	defer os.Remove(dst)
	sum, err := pkgmgr.Download(ctx, c.Client, zipURL, dst, func(int64, int64) {})
	if err != nil {
		return nil, fmt.Errorf("baixar o Apache da build %s: %w", b.build, err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		return nil, err
	}
	txt, err := c.get(ctx, zipURL+".txt", 64<<10)
	if err != nil {
		return nil, fmt.Errorf("SHA-256 do Apache da build %s: %w", b.build, err)
	}
	m := sha256Lounge.FindSubmatch(txt)
	if m == nil {
		return nil, fmt.Errorf("%s.txt sem a linha do SHA-256", zipURL)
	}
	if !strings.EqualFold(string(m[1]), sum) {
		return nil, fmt.Errorf("o SHA-256 do %s (%s) não confere com o .txt do Lounge (%s)", path.Base(zipURL), sum, m[1])
	}

	id := "apache-" + b.versao + "-vs18-x64"
	nota := atual.Notes
	if mb := buildCatalogo.FindStringSubmatch(atual.URL); mb != nil {
		nota = strings.NewReplacer(atual.URL, zipURL, atual.ID, id, "build "+mb[1], "build "+b.build).Replace(nota)
	}
	var entrada bytes.Buffer
	enc := json.NewEncoder(&entrada)
	enc.SetEscapeHTML(false) // a nota tem "->"
	enc.SetIndent("", "  ")
	if err := enc.Encode(pkgmgr.Package{
		ID: id, Kind: runtime.Apache, Version: b.versao, URL: zipURL,
		SHA256: strings.ToLower(sum), Compiler: "VS18", Arch: "x64", Notes: nota,
	}); err != nil {
		return nil, err
	}
	estado := "ainda responde"
	if err := c.sondar(ctx, atual.URL); err != nil {
		estado = fmt.Sprintf("não responde mais (%v)", err)
	}

	var corpo strings.Builder
	fmt.Fprintf(&corpo, "O Apache Lounge publicou o Apache %s, build %s. O catálogo tem `%s`, e o link dele %s.\n\n", b.versao, b.build, atual.ID, estado)
	fmt.Fprintf(&corpo, "| | |\n|---|---|\n| Arquivo | %s |\n| Tamanho | %d bytes |\n| SHA-256 | `%s` (conferido com o .txt do Lounge) |\n\n", zipURL, info.Size(), strings.ToLower(sum))
	fmt.Fprintf(&corpo, "Entrada para `internal/pkgmgr/catalog.json`, no lugar da atual:\n\n```json\n%s```\n\n", entrada.String())
	corpo.WriteString("Confira na página do Lounge se a nota continua certa (VC++ Redistributable exigido e a raiz do zip). Depois, suba o `serial` e publique o catálogo.")
	return []Achado{{
		Titulo: fmt.Sprintf("catálogo: Apache %s build %s no Apache Lounge", b.versao, b.build),
		Corpo:  corpo.String(),
		Unico:  true,
	}}, nil
}

// ultimaBuild devolve a build de maior número na página. Os números de build
// do Lounge crescem com o tempo (AAMMDD), inclusive entre versões.
func ultimaBuild(pagina []byte) (buildApache, bool) {
	var melhor buildApache
	maior := -1
	for _, m := range buildLounge.FindAllSubmatch(pagina, -1) {
		n, err := strconv.Atoi(string(m[2]))
		if err != nil || n <= maior {
			continue
		}
		maior = n
		melhor = buildApache{versao: string(m[1]), build: string(m[2]), caminho: strings.TrimSuffix(string(m[0]), `"`)}
	}
	return melhor, maior >= 0
}

// fontes testa a url e os mirrors de cada pacote com um GET de 1 byte.
// Pacote sem nenhuma fonte boa vira issue; fonte caída com outra boa só vai
// para o log, porque o download já cai para a que responde. pular é o pacote
// que já tem issue por outro motivo.
func (c Checagem) fontes(ctx context.Context, cat pkgmgr.Catalog, pular string) []Achado {
	var achados []Achado
	for _, p := range cat.Packages {
		if p.ID == pular {
			fmt.Fprintf(c.Log, "%s: fontes não conferidas; a issue da build nova do Apache já cobre o link antigo\n", p.ID)
			continue
		}
		errPrincipal := c.sondar(ctx, p.URL)
		var falhas []string
		if errPrincipal != nil {
			falhas = append(falhas, fmt.Sprintf("- %s: %v", p.URL, errPrincipal))
		}
		// Todo mirror é sondado, mesmo com o principal vivo: senão um mirror
		// morto só seria notado no dia em que o principal também caísse.
		mirror := ""
		var errMirrors []error
		for _, m := range p.Mirrors {
			err := c.sondar(ctx, m)
			errMirrors = append(errMirrors, err)
			if err != nil {
				falhas = append(falhas, fmt.Sprintf("- %s: %v", m, err))
			} else if mirror == "" {
				mirror = m
			}
		}
		if errPrincipal == nil || mirror != "" {
			if errPrincipal != nil {
				fmt.Fprintf(c.Log, "%s: o link principal falhou (%v); o mirror %s responde\n", p.ID, errPrincipal, mirror)
			}
			for i, err := range errMirrors {
				if err != nil && errPrincipal == nil {
					fmt.Fprintf(c.Log, "%s: o mirror %s falhou (%v); o link principal responde\n", p.ID, p.Mirrors[i], err)
				} else if err != nil {
					fmt.Fprintf(c.Log, "%s: o mirror %s falhou (%v); o mirror %s responde\n", p.ID, p.Mirrors[i], err, mirror)
				}
			}
			continue
		}
		achados = append(achados, Achado{
			Titulo: "catálogo: " + p.ID + " sem fonte disponível",
			Corpo: fmt.Sprintf("Nenhuma fonte deste pacote entregou o arquivo, com %d tentativas em cada:\n\n%s\n\n"+
				"A instalação deste pacote falha em todo app até o catálogo publicado ganhar um link que responda.", tentativas, strings.Join(falhas, "\n")),
		})
	}
	return achados
}

// sondar pede o primeiro byte de u, seguindo redirecionamentos, e tenta de
// novo nas falhas. Vale 200 ou 206 que não seja uma página HTML (o Lounge
// responde HTML quando a build sumiu).
func (c Checagem) sondar(ctx context.Context, u string) error {
	var err error
	for i := range tentativas {
		if i > 0 {
			c.esperar(ctx)
		}
		if err = c.sondarUmaVez(ctx, u); err == nil {
			return nil
		}
	}
	return err
}

func (c Checagem) sondarUmaVez(ctx context.Context, u string) error {
	ctx, cancel := context.WithTimeout(ctx, prazoSonda)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", agenteChecagem)
	req.Header.Set("Range", "bytes=0-0")
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	tipo := resp.Header.Get("Content-Type")
	switch {
	case resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent:
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	case strings.HasPrefix(tipo, "text/html"):
		return fmt.Errorf("devolveu %s em vez do arquivo", tipo)
	}
	return nil
}

// get faz o GET de u com limite de tamanho, tentando de novo nas falhas.
func (c Checagem) get(ctx context.Context, u string, limit int64) ([]byte, error) {
	var err error
	for i := range tentativas {
		if i > 0 {
			c.esperar(ctx)
		}
		var raw []byte
		if raw, err = pkgmgr.GetLimited(ctx, c.Client, u, limit, agenteChecagem); err == nil {
			return raw, nil
		}
	}
	return nil, err
}

func (c Checagem) esperar(ctx context.Context) {
	t := time.NewTimer(c.Pausa)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// issueListada é uma linha do `gh issue list --json title`.
type issueListada struct {
	Title string `json:"title"`
}

// abrirIssues abre uma issue por achado novo, com o rótulo da checagem. A
// comparação de títulos é exata e feita aqui, sobre a lista de issues do
// rótulo: a busca do GitHub trata "catálogo:" como qualificador.
func abrirIssues(gh func(args ...string) ([]byte, error), repo string, achados []Achado, log io.Writer) error {
	if len(achados) == 0 {
		return nil
	}
	if _, err := gh("label", "create", rotuloChecagem, "--repo", repo, "--color", "C5DEF5",
		"--description", "Checagem diária do catálogo de pacotes", "--force"); err != nil {
		return fmt.Errorf("gh label create: %w", err)
	}
	for _, a := range achados {
		estado := "open"
		if a.Unico {
			estado = "all"
		}
		raw, err := gh("issue", "list", "--repo", repo, "--label", rotuloChecagem, "--state", estado, "--json", "title", "--limit", "500")
		if err != nil {
			return fmt.Errorf("gh issue list: %w", err)
		}
		var lista []issueListada
		if err := json.Unmarshal(raw, &lista); err != nil {
			return fmt.Errorf("gh issue list: %w", err)
		}
		if slices.ContainsFunc(lista, func(i issueListada) bool {
			return i.Title == a.Titulo
		}) {
			fmt.Fprintf(log, "já existe issue: %s\n", a.Titulo)
			continue
		}
		if _, err := gh("issue", "create", "--repo", repo, "--title", a.Titulo, "--body", a.Corpo, "--label", rotuloChecagem); err != nil {
			return fmt.Errorf("gh issue create %q: %w", a.Titulo, err)
		}
		fmt.Fprintf(log, "issue aberta: %s\n", a.Titulo)
	}
	return nil
}
