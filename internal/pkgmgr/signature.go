package pkgmgr

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
)

// VerifySignature confere a assinatura ed25519 (base64 padrão) dos bytes
// exatos de body. Espaços ao redor da assinatura são ignorados: o .sig
// publicado termina em quebra de linha. É o formato do latest.json.sig do
// update e do catalog.json.sig do catálogo.
func VerifySignature(pub ed25519.PublicKey, body, sig []byte) error {
	raw, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(sig)))
	if err != nil {
		return fmt.Errorf("assinatura ilegível: %w", err)
	}
	if !ed25519.Verify(pub, body, raw) {
		return errors.New("assinatura inválida")
	}
	return nil
}
