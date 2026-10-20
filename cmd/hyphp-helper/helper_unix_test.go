//go:build darwin || linux

package main

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
)

// certPEM gera um certificado autoassinado; isCA decide se ele é de uma CA.
func certPEM(t *testing.T, isCA bool) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "teste"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  isCA,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// Rodando como root, confiar em qualquer arquivo indicado abriria a confiança
// do sistema: só um rootCA.pem de CA passa.
func TestCATrustRecusaOQueNaoEUmaCA(t *testing.T) {
	dir := t.TempDir()
	escrever := func(nome string, conteudo []byte) string {
		p := filepath.Join(t.TempDir(), nome)
		if err := os.WriteFile(p, conteudo, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for nome, cert := range map[string]string{
		"nome errado": escrever("outro.pem", certPEM(t, true)),
		"não é CA":    escrever("rootCA.pem", certPEM(t, false)),
		"não é PEM":   escrever("rootCA.pem", []byte("lixo")),
		"relativo":    "rootCA.pem",
		"não existe":  filepath.Join(dir, "rootCA.pem"),
	} {
		if code, _ := run([]string{"ca-trust", "--cert", cert}); code != exitUsage {
			t.Errorf("%s: code = %d, quer %d", nome, code, exitUsage)
		}
	}
}
