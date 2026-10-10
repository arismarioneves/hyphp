package webserver

import (
	_ "embed"
	"encoding/json"
	"strings"

	"hyphp/internal/i18n"
)

// defaultIndexHTML é a página do host default — a que responde quando o
// domínio pedido não casa com nenhum projeto. Vive aqui, e não dentro de
// apache/ ou nginx/, porque é a MESMA página nos dois: duplicá-la garantiria
// que um dia só uma das cópias fosse atualizada.
//
// Os textos são marcadores {{chave}} resolvidos pelo catálogo do i18n.
//
//go:embed default-index.html
var defaultIndexHTML string

// pageKeys são as chaves do catálogo que a página usa, cada uma no marcador
// {{chave}} do HTML.
var pageKeys = []string{"page.title", "page.sub", "page.lead", "page.step1", "page.step2", "page.step3", "page.lista"}

// scriptKeys são os textos do script da página. Vão como JSON no marcador
// {{page.textos}}: o script escolhe a mensagem pelo host e não tem texto
// próprio. {host} e {dominio} nesses textos nunca ficam dentro de uma tag,
// porque o script monta cada pedaço entre eles como HTML separado.
var scriptKeys = []string{
	"page.local", "page.semProjetos", "page.subSemWildcard", "page.subSemDNS",
	"page.testSemProjeto", "page.foraDoTest", "page.naoAbreHosts", "page.naoAbreDNS",
}

// DefaultIndexHTML monta a página do host default no idioma atual. É chamada
// a cada render do Reconcile, que roda de novo quando o idioma muda — por isso
// o texto é resolvido aqui, e não uma vez no init. Devolve um slice novo porque
// o chamador o coloca num map[string][]byte que atravessa render.WriteFiles.
//
// Os textos do catálogo entram sem escape: são HTML confiável (trazem <code> e
// <strong>), nunca dado do usuário.
func DefaultIndexHTML() []byte {
	pairs := make([]string, 0, 2*(len(pageKeys)+2))
	pairs = append(pairs, "{{lang}}", string(i18n.Current()))
	for _, k := range pageKeys {
		pairs = append(pairs, "{{"+k+"}}", i18n.T(k))
	}
	textos := make(map[string]string, len(scriptKeys))
	for _, k := range scriptKeys {
		textos[k] = i18n.T(k)
	}
	// map de strings sempre serializa; o json.Marshal ordena as chaves (a
	// página sai igual a cada render) e escapa <, > e &, então o JSON não
	// fecha o <script> que o contém.
	js, _ := json.Marshal(textos)
	pairs = append(pairs, "{{page.textos}}", string(js))
	return []byte(strings.NewReplacer(pairs...).Replace(defaultIndexHTML))
}
