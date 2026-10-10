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

	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
)

// sistemaFalso aponta os arquivos de sistema para uma pasta temporária: nenhum
// teste escreve em /etc nem mexe no cache de DNS da máquina.
func sistemaFalso(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	resolver, pathsD, flush := netcfg.ResolverRulePath, paths.PathsDFile, flushDNS
	netcfg.ResolverRulePath = filepath.Join(dir, "resolver", "test")
	paths.PathsDFile = filepath.Join(dir, "paths.d", "hyphp")
	flushDNS = func() {}
	t.Cleanup(func() { netcfg.ResolverRulePath, paths.PathsDFile, flushDNS = resolver, pathsD, flush })
}

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

func TestResolverWriteGravaARegraDoHyPHP(t *testing.T) {
	sistemaFalso(t)
	if code, err := run([]string{"resolver-write", "--port", "15353"}); code != exitOK {
		t.Fatalf("code = %d, err = %v", code, err)
	}
	raw, err := os.ReadFile(netcfg.ResolverRulePath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(raw), netcfg.RenderResolverRule(15353); got != want {
		t.Fatalf("regra = %q, quer %q", got, want)
	}
}

// Porta privilegiada não serve: o resolvedor do app roda sem root e não
// consegue escutar abaixo de 1024 em 127.0.0.1.
func TestResolverWriteRecusaPortaPrivilegiada(t *testing.T) {
	sistemaFalso(t)
	if code, _ := run([]string{"resolver-write", "--port", "53"}); code != exitUsage {
		t.Fatalf("code = %d, quer %d", code, exitUsage)
	}
	if _, err := os.Stat(netcfg.ResolverRulePath); !os.IsNotExist(err) {
		t.Fatalf("a regra foi gravada mesmo recusada: %v", err)
	}
}

// Cada linha do paths.d vira uma entrada do PATH de todo shell de login: só a
// pasta cli que o app criou passa.
func TestPathsWriteAceitaSoAPastaCli(t *testing.T) {
	sistemaFalso(t)
	base := t.TempDir()
	cli := filepath.Join(base, "cli")
	if err := os.Mkdir(cli, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{
		"cli",                             // relativo
		base + "/x/../cli",                // não limpo (Join limparia)
		filepath.Join(base, "bin"),        // nome errado
		filepath.Join(t.TempDir(), "cli"), // não existe
		cli + "\n/usr/local/evil",         // quebra de linha
	} {
		if code, _ := run([]string{"paths-write", "--dir", dir}); code != exitUsage {
			t.Errorf("--dir %q: code = %d, quer %d", dir, code, exitUsage)
		}
	}
	if _, err := os.Stat(paths.PathsDFile); !os.IsNotExist(err) {
		t.Fatalf("paths.d gravado com --dir recusado: %v", err)
	}
	if code, err := run([]string{"paths-write", "--dir", cli}); code != exitOK {
		t.Fatalf("pasta cli válida: code = %d, err = %v", code, err)
	}
	raw, err := os.ReadFile(paths.PathsDFile)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(raw), paths.PathsDContent(cli); got != want {
		t.Fatalf("paths.d = %q, quer %q", got, want)
	}
}

// Rodando como root, marcar como confiável qualquer arquivo indicado abriria o
// keychain do sistema: só um rootCA.pem de CA passa.
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

// A regra de outro programa (Valet) fica; a do HyPHP e o paths.d saem.
func TestUninstallPreservaRegraDeOutroPrograma(t *testing.T) {
	for _, c := range []struct {
		nome  string
		regra string
		sobra bool
	}{
		{"regra do HyPHP", netcfg.RenderResolverRule(15353), false},
		{"regra do Valet", "nameserver 127.0.0.1\n", true},
	} {
		t.Run(c.nome, func(t *testing.T) {
			sistemaFalso(t)
			for p, conteudo := range map[string]string{netcfg.ResolverRulePath: c.regra, paths.PathsDFile: "/x/cli\n"} {
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(conteudo), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if code, err := run([]string{"uninstall"}); code != exitOK {
				t.Fatalf("code = %d, err = %v", code, err)
			}
			if _, err := os.Stat(netcfg.ResolverRulePath); (err == nil) != c.sobra {
				t.Fatalf("regra presente = %v, quer %v", err == nil, c.sobra)
			}
			if _, err := os.Stat(paths.PathsDFile); !os.IsNotExist(err) {
				t.Fatalf("paths.d ficou: %v", err)
			}
		})
	}
}
