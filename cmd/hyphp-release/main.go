// Command hyphp-release publica uma versão do HyPHP na pasta releases/ do
// site (C:\DEV\hyphp-web\docs\contrato-releases.md).
//
//	go run ./cmd/hyphp-release -gerar-chave
//	go run ./cmd/hyphp-release -nota "Corrige X" -nota "Adiciona Y"
//	go run ./cmd/hyphp-release -so-js
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hyphp/internal/update"
	"hyphp/internal/version"
)

type notas []string

func (n *notas) String() string     { return strings.Join(*n, " | ") }
func (n *notas) Set(v string) error { *n = append(*n, v); return nil }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "hyphp-release:", err)
		os.Exit(1)
	}
}

func run() error {
	home, _ := os.UserHomeDir()
	var (
		gerar = flag.Bool("gerar-chave", false, "gera o par ed25519 e imprime a chave pública")
		soJS  = flag.Bool("so-js", false, "só reescreve releases/releases.js a partir do index.json")
		web   = flag.String("web", `C:\DEV\hyphp-web`, "raiz do repositório do site")
		inst  = flag.String("instalador", filepath.Join("bin", "hyphp-amd64-installer.exe"), "instalador gerado por wails3 task windows:package")
		data  = flag.String("data", time.Now().Format(time.DateOnly), "data da publicação (AAAA-MM-DD)")
		chave = flag.String("chave", filepath.Join(home, ".hyphp", "release-ed25519.key"), "chave privada (seed em hex)")
		ns    notas
	)
	flag.Var(&ns, "nota", "item das notas da versão (repetível)")
	flag.Parse()

	if *gerar {
		return generateKey(*chave)
	}
	if *soJS {
		return Regenerate(*web)
	}
	if err := checkVersions(".", version.Current); err != nil {
		return err
	}
	priv, err := loadKey(*chave)
	if err != nil {
		return err
	}
	// Assinar com uma chave que o binário não conhece publicaria um manifesto
	// que todo app instalado recusa — em silêncio, do lado do usuário.
	if !priv.Public().(ed25519.PublicKey).Equal(update.PublicKey) {
		return fmt.Errorf("a chave %s não corresponde a update.PublicKey embutida no app", *chave)
	}
	rel, err := Publish(Options{WebDir: *web, Installer: *inst, Version: version.Current, Date: *data, Notes: ns, Key: priv})
	if err != nil {
		return err
	}
	fmt.Printf("publicada %s em %s\n  %s\n  %d bytes, sha256 %s\n", rel.Version, *web, rel.WindowsAMD64.Path, rel.WindowsAMD64.Size, rel.WindowsAMD64.SHA256)
	fmt.Println("próximo passo: no hyphp-web, commitar releases/ e dar push no main; a Hostinger publica o main")
	return nil
}

// generateKey recusa sobrescrever: trocar a chave deixaria todo app já
// instalado sem conseguir validar nenhum manifesto novo.
func generateKey(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s já existe; trocar a chave corta o update de quem já instalou", path)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Printf("chave privada: %s (faça backup fora desta máquina)\nchave pública (cole em internal/update/key.go): %s\n", path, hex.EncodeToString(pub))
	return nil
}

func loadKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s ausente; rode com -gerar-chave uma única vez, ou restaure o backup", path)
	}
	if err != nil {
		return nil, err
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("%s não contém uma seed ed25519 em hex", path)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}
