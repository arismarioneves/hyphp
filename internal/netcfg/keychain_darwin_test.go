package netcfg

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A saída do `security find-certificate -Z` lista um "SHA-1 hash:" por
// certificado; só o hash exato conta, sem diferenciar caixa.
func TestKeychainTemOSHA1(t *testing.T) {
	out := "keychain: \"/Library/Keychains/System.keychain\"\nSHA-256 hash: 11AA\nSHA-1 hash: C0B4358638C552DBEB105E0F22D2B59A1B89B2F4\n" +
		"keychain: \"/Library/Keychains/System.keychain\"\nSHA-1 hash: 6204540F69BB8898DC43331C1B560ACFE3110110\n"
	if !keychainHasSHA1(out, "c0b4358638c552dbeb105e0f22d2b59a1b89b2f4") {
		t.Fatal("não achou o primeiro certificado")
	}
	if keychainHasSHA1(out, "11AA") {
		t.Fatal("confundiu SHA-256 com SHA-1")
	}
	if keychainHasSHA1(out, "0000000000000000000000000000000000000000") {
		t.Fatal("achou um certificado que não está na lista")
	}
}

// Sem o rootCA.pem não há o que confiar; com uma CA que só existe em disco
// (nunca foi para o keychain do sistema) o HTTPS continua pendente.
func TestCAInstaladaNoMacExigeOKeychain(t *testing.T) {
	root := t.TempDir()
	m := Mkcert{Exe: "/opt/homebrew/opt/mkcert/bin/mkcert", CARoot: root}
	if ok, err := m.CAInstalled(); ok || err != nil {
		t.Fatalf("sem rootCA.pem: CAInstalled = (%v, %v), quer (false, nil)", ok, err)
	}
	if err := os.WriteFile(filepath.Join(root, "rootCA.pem"), selfSignedPEM(t, time.Now().Add(time.Hour)), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.CAInstalled(); ok || err != nil {
		t.Fatalf("CA fora do keychain: CAInstalled = (%v, %v), quer (false, nil)", ok, err)
	}
}

// O SHA-1 é o que o helper usa para apagar a CA do keychain: arquivo que não
// é certificado não pode virar um hash.
func TestCertSHA1RecusaPEMQueNaoECertificado(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rootCA.pem")
	if err := os.WriteFile(p, []byte("-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CertSHA1(p); err == nil {
		t.Fatal("CertSHA1 aceitou uma chave privada")
	}
}
