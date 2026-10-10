package pkgmgr

import (
	"crypto/ed25519"
	"encoding/hex"
)

// catalogKeyHex é a chave pública que confere o catalog.json.sig. A privada
// correspondente fica fora dos repositórios e assina o catálogo na
// publicação; foi gerada uma única vez por
// `go run ./cmd/hyphp-release -gerar-chave-catalogo`. É separada da chave do
// update, mas vale o mesmo: quem tiver a privada consegue fazer o app baixar
// outro binário. Trocar esta constante faz as versões instaladas ignorarem o
// catálogo remoto e ficarem com o embutido.
const catalogKeyHex = "dbc848e64bf6d07fe693f3d466aac5af19df523d2e653b7379d06eb97af9bd85"

// CatalogKey é catalogKeyHex decodificada.
var CatalogKey = mustCatalogKey(catalogKeyHex)

// mustCatalogKey entra em pânico com chave malformada: é constante do build.
func mustCatalogKey(h string) ed25519.PublicKey {
	raw, err := hex.DecodeString(h)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		panic("pkgmgr: chave pública do catálogo inválida")
	}
	return ed25519.PublicKey(raw)
}
