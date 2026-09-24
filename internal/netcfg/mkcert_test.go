package netcfg

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestCertBaseName(t *testing.T) {
	a := certBaseName(normalizeDomains([]string{"acme.test", "*.acme.test"}))
	b := certBaseName(normalizeDomains([]string{"*.ACME.test", " acme.test "}))
	c := certBaseName(normalizeDomains([]string{"acme.test"}))

	if len(a) != 12 {
		t.Fatalf("nome deve ter 12 hex, veio %q", a)
	}
	if a != b {
		t.Fatalf("ordem/caixa/espaços não podem mudar o nome: %q != %q", a, b)
	}
	if a == c {
		t.Fatalf("conjuntos diferentes de domínios não podem colidir: %q", a)
	}
}

// selfSignedPEM gera um certificado auto-assinado que expira em notAfter, só para o teste.
func selfSignedPEM(t *testing.T, notAfter time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "hyphp test"},
		DNSNames:     []string{"hello.test"},
		NotBefore:    notAfter.Add(-365 * 24 * time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestNeedsRenewal(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		pem  []byte
		want bool
	}{
		{"expira em 60 dias: mantém", selfSignedPEM(t, now.Add(60*24*time.Hour)), false},
		{"expira em 31 dias: mantém", selfSignedPEM(t, now.Add(31*24*time.Hour)), false},
		{"expira em 29 dias: renova", selfSignedPEM(t, now.Add(29*24*time.Hour)), true},
		{"já expirado: renova", selfSignedPEM(t, now.Add(-time.Hour)), true},
		{"PEM inválido: renova", []byte("lixo"), true},
		{"PEM de outro tipo: renova", pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte{1, 2, 3}}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsRenewal(tt.pem, now); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
