package stack

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hyphp/internal/i18n"
	"hyphp/internal/netcfg"
)

// A CA criada pelo mkcert como usuário só existe em disco até o helper a pôr
// no keychain: o HTTPS segue pendente, e o botão continua oferecido.
func TestCASoEmDiscoContinuaPendenteNoMac(t *testing.T) {
	root := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "mkcert teste"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rootCA.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Stack{d: Deps{Mkcert: netcfg.Mkcert{Exe: "/opt/homebrew/opt/mkcert/bin/mkcert", CARoot: root}}}
	if _, warns := s.tlsIssuer(); len(warns) != 1 || warns[0].Code != "ca-pending" {
		t.Fatalf("warnings = %+v, quer um ca-pending", warns)
	}
}

// Sem mkcert no Mac o aviso aponta o Runtimes (Homebrew), não a pasta bin/
// do Windows.
func TestSemMkcertNoMac(t *testing.T) {
	s := &Stack{d: Deps{Mkcert: netcfg.Mkcert{}}}
	if _, warns := s.tlsIssuer(); len(warns) != 1 || warns[0].Message != i18n.T("warn.mkcertMissingBrew") {
		t.Fatalf("warnings = %+v", warns)
	}
}
