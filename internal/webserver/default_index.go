package webserver

import (
	_ "embed"
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
var pageKeys = []string{"page.title", "page.sub", "page.lead", "page.step1", "page.step2", "page.step3"}

// DefaultIndexHTML monta a página do host default no idioma atual. É chamada
// a cada render do Reconcile, que roda de novo quando o idioma muda — por isso
// o texto é resolvido aqui, e não uma vez no init. Devolve um slice novo porque
// o chamador o coloca num map[string][]byte que atravessa render.WriteFiles.
//
// Os textos do catálogo entram sem escape: são HTML confiável (trazem <code> e
// <strong>), nunca dado do usuário.
func DefaultIndexHTML() []byte {
	pairs := make([]string, 0, 2*(len(pageKeys)+1))
	pairs = append(pairs, "{{lang}}", string(i18n.Current()))
	for _, k := range pageKeys {
		pairs = append(pairs, "{{"+k+"}}", i18n.T(k))
	}
	return []byte(strings.NewReplacer(pairs...).Replace(defaultIndexHTML))
}
