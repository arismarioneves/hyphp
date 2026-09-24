package webserver

import (
	_ "embed"
	"slices"
)

// defaultIndexHTML é a página do host default — a que responde quando o
// domínio pedido não casa com nenhum projeto. Vive aqui, e não dentro de
// apache/ ou nginx/, porque é a MESMA página nos dois: duplicá-la garantiria
// que um dia só uma das cópias fosse atualizada.
//
//go:embed default-index.html
var defaultIndexHTML []byte

// DefaultIndexHTML devolve uma cópia da página do host default. Cópia porque o
// chamador coloca o slice num map[string][]byte que atravessa render.WriteFiles.
func DefaultIndexHTML() []byte { return slices.Clone(defaultIndexHTML) }
