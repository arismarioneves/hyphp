// Command hyphp-release é a ferramenta dos jobs do .github/workflows/build.yml:
// confere a versão antes de compilar, confere o instalador do Windows,
// publica a release no GitHub com os dois instaladores e registra a versão no
// histórico do site (releases/index.json e releases.js).
//
//	go run ./cmd/hyphp-release -verificar -tag v3.1.0     (-tag "" no ensaio)
//	go run ./cmd/hyphp-release -conferir-instalador bin/hyphp-amd64-installer.exe
//	go run ./cmd/hyphp-release -publicar -tag v3.1.0 -instalador … -dmg … -install … -uninstall … -web web
//	go run ./cmd/hyphp-release -ensaio -instalador … -dmg …
//	go run ./cmd/hyphp-release -gerar-chave
//	go run ./cmd/hyphp-release -gerar-chave-catalogo
//	go run ./cmd/hyphp-release -catalogo -web <checkout do site>
//	go run ./cmd/hyphp-release -so-js -web <checkout do site>
//
// A chave de assinatura vem só da variável de ambiente HYPHP_RELEASE_KEY;
// -gerar-chave a cria uma única vez. A do catálogo, separada, vem de
// HYPHP_CATALOG_KEY e é criada por -gerar-chave-catalogo.
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
	"time"

	"hyphp/internal/pkgmgr"
	"hyphp/internal/update"
	"hyphp/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "hyphp-release:", err)
		os.Exit(1)
	}
}

func run() error {
	home, _ := os.UserHomeDir()
	var (
		gerar         = flag.Bool("gerar-chave", false, "gera o par ed25519 em ~/.hyphp e imprime a chave pública")
		gerarCatalogo = flag.Bool("gerar-chave-catalogo", false, "gera o par ed25519 do catálogo em ~/.hyphp e imprime a chave pública")
		soJS          = flag.Bool("so-js", false, "só reescreve releases/releases.js a partir do index.json")
		verificar     = flag.Bool("verificar", false, "confere versões, tag, notas e a última release (job verificar)")
		conferir      = flag.String("conferir-instalador", "", "confere a ProductVersion do instalador do Windows")
		publicar      = flag.Bool("publicar", false, "publica a release e grava o histórico do site (job publicar)")
		ensaio        = flag.Bool("ensaio", false, "monta e assina o latest.json em memória, sem publicar")
		catalogo      = flag.Bool("catalogo", false, "assina o internal/pkgmgr/catalog.json e o grava em <web>/catalogo (workflow catalogo.yml)")
		tag           = flag.String("tag", "", "tag v<versão> da release")
		web           = flag.String("web", "", "checkout do repositório do site")
		inst          = flag.String("instalador", "", "instalador do Windows (bin/hyphp-amd64-installer.exe)")
		dmg           = flag.String("dmg", "", "dmg do Mac (bin/HyPHP.dmg)")
		install       = flag.String("install", "", "build/darwin/install.sh")
		uninstall     = flag.String("uninstall", "", "build/darwin/uninstall.sh")
		data          = flag.String("data", time.Now().Format(time.DateOnly), "data da publicação (AAAA-MM-DD)")
	)
	flag.Parse()

	modos := 0
	for _, on := range []bool{*gerar, *gerarCatalogo, *soJS, *verificar, *conferir != "", *publicar, *ensaio, *catalogo} {
		if on {
			modos++
		}
	}
	if modos != 1 {
		return errors.New("escolha um modo: -verificar, -conferir-instalador, -publicar, -ensaio, -catalogo, -gerar-chave, -gerar-chave-catalogo ou -so-js")
	}
	files := Files{Installer: *inst, DMG: *dmg, Install: *install, Uninstall: *uninstall}

	switch {
	case *gerar:
		return generateKey(filepath.Join(home, ".hyphp", "release-ed25519.key"), "internal/update/key.go", releaseKeyEnv)
	case *gerarCatalogo:
		return generateKey(filepath.Join(home, ".hyphp", "catalogo-ed25519.key"), "internal/pkgmgr/key.go", catalogKeyEnv)
	case *soJS:
		if *web == "" {
			return errors.New("-so-js exige -web <checkout do site>")
		}
		return Regenerate(*web)
	case *conferir != "":
		return checkInstallerVersion(*conferir, version.Current)
	case *verificar:
		if err := verify(".", version.Current, *tag, update.GitHubRepo, runGH); err != nil {
			return err
		}
		// O build.yml acrescenta esta linha ao $GITHUB_OUTPUT do job.
		fmt.Printf("versao=%s\n", version.Current)
		return nil
	case *ensaio:
		key, err := signingKey(releaseKeyEnv, os.Getenv(releaseKeyEnv), update.PublicKey)
		if err != nil {
			return err
		}
		l, err := rehearse(version.Current, *data, files, key, update.PublicKey)
		if err != nil {
			return err
		}
		fmt.Printf("ensaio ok: latest.json da %s assinado e conferido com a chave do app; nada publicado\n  %s, %d bytes\n  %s, %d bytes\n",
			l.Version, l.WindowsAMD64.Path, l.WindowsAMD64.Size, l.DarwinARM64.Path, l.DarwinARM64.Size)
		return nil
	case *catalogo:
		if *web == "" {
			return errors.New("-catalogo exige -web <checkout do site>")
		}
		key, err := signingKey(catalogKeyEnv, os.Getenv(catalogKeyEnv), pkgmgr.CatalogKey)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(filepath.Join("internal", "pkgmgr", "catalog.json"))
		if err != nil {
			return err
		}
		c, err := publishCatalog(raw, *web, key)
		if err != nil {
			return err
		}
		// O catalogo.yml usa esta linha na mensagem do commit.
		fmt.Printf("serial=%d\n", c.Serial)
		return nil
	}

	// -publicar: as conferências do job verificar de novo, agora com os
	// arquivos que os jobs de build entregaram.
	if *tag == "" {
		return errors.New("-publicar exige -tag v<versão>")
	}
	if *web == "" {
		return errors.New("-publicar exige -web <checkout do site>")
	}
	if err := verify(".", version.Current, *tag, update.GitHubRepo, runGH); err != nil {
		return err
	}
	key, err := signingKey(releaseKeyEnv, os.Getenv(releaseKeyEnv), update.PublicKey)
	if err != nil {
		return err
	}
	notes, err := readNotes(".", version.Current)
	if err != nil {
		return err
	}
	if err := checkInstallerVersion(files.Installer, version.Current); err != nil {
		return err
	}
	rel, err := Publish(Options{WebDir: *web, Repo: update.GitHubRepo, Version: version.Current, Date: *data, Notes: notes, Files: files, Key: key, GH: runGH})
	if err != nil {
		return err
	}
	fmt.Printf("publicada %s em https://github.com/%s/releases/tag/v%s\n", rel.Version, update.GitHubRepo, rel.Version)
	for _, a := range []*update.Artifact{rel.WindowsAMD64, rel.DarwinARM64} {
		fmt.Printf("  %s, %d bytes, sha256 %s\n", a.Path, a.Size, a.SHA256)
	}
	return nil
}

// generateKey recusa sobrescrever: trocar a chave deixaria todo app já
// instalado sem conseguir validar nada assinado com a nova. keyFile é onde a
// chave pública vai no código e env, o segredo do workflow que guarda a seed.
func generateKey(path, keyFile, env string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s já existe; trocar a chave corta quem já instalou", path)
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
	fmt.Printf("chave privada: %s (faça backup fora desta máquina)\n", path)
	fmt.Printf("chave pública (cole em %s): %s\n", keyFile, hex.EncodeToString(pub))
	fmt.Printf("guarde a seed de %s como o segredo %s do workflow\n", path, env)
	return nil
}
