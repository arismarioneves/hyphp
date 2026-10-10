package update

import (
	"crypto/ed25519"
	"encoding/hex"
)

// publicKeyHex é a chave pública que valida latest.json.sig.
//
// A privada correspondente fica fora dos repositórios e assina o latest.json
// na publicação (cmd/hyphp-release); foi gerada uma única vez por
// `go run ./cmd/hyphp-release -gerar-chave`. Trocar esta
// constante deixa todo app já instalado sem aceitar nenhum manifesto novo: a
// saída passa a ser o usuário reinstalar à mão.
const publicKeyHex = "ca9f906be2fcd20120291290e72cc114e2bdd2e724d663e0dc25dc8fc4961d79"

// PublicKey é publicKeyHex decodificada.
var PublicKey = mustKey(publicKeyHex)

// mustKey entra em pânico com chave malformada: é constante do build, e um
// binário que não consegue validar manifesto nenhum não deve sair do forno.
func mustKey(h string) ed25519.PublicKey {
	raw, err := hex.DecodeString(h)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		panic("update: chave pública embutida inválida")
	}
	return ed25519.PublicKey(raw)
}
