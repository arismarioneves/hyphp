package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"gopkg.in/yaml.v3"

	"hyphp/internal/update"
)

// Options descreve uma publicação.
type Options struct {
	WebDir    string // raiz do repositório hyphp-web
	Installer string // instalador gerado por `wails3 task windows:package`
	Version   string
	Date      string // AAAA-MM-DD
	Notes     []string
	Key       ed25519.PrivateKey
}

// Publish copia o instalador para releases/<v>/, atualiza index.json e
// releases.js e grava latest.json assinado. Tudo o que pode recusar é checado
// antes de escrever o primeiro byte: uma publicação recusada não deixa rastro.
func Publish(o Options) (update.Release, error) {
	dir := releasesDir(o.WebDir)
	idx, err := readIndex(filepath.Join(dir, "index.json"))
	if err != nil {
		return update.Release{}, err
	}
	// Arquivo de versão publicada é imutável: o CDN guarda o .exe por um ano,
	// e republicar serviria o instalador velho com o sha256 novo.
	verDir := filepath.Join(dir, o.Version)
	if _, err := os.Stat(verDir); err == nil {
		return update.Release{}, fmt.Errorf("versão %s já publicada em %s; correção sai como versão nova", o.Version, verDir)
	}
	if len(idx.Releases) > 0 {
		c, err := update.Compare(o.Version, idx.Releases[0].Version)
		if err != nil {
			return update.Release{}, err
		}
		if c <= 0 {
			return update.Release{}, fmt.Errorf("versão %s não é maior que a última publicada (%s)", o.Version, idx.Releases[0].Version)
		}
	}
	size, sum, err := hashFile(o.Installer)
	if err != nil {
		return update.Release{}, err
	}
	name := "hyphp-" + o.Version + "-windows-amd64-setup.exe"
	rel := update.Release{
		Version: o.Version,
		Date:    o.Date,
		Notes:   append([]string{}, o.Notes...),
		WindowsAMD64: &update.Artifact{
			Path:   o.Version + "/" + name,
			Size:   size,
			SHA256: sum,
		},
	}
	// A mesma validação que o app faz: o site nunca serve o que ele recusaria.
	if err := rel.Validate(); err != nil {
		return update.Release{}, err
	}

	if err := os.MkdirAll(verDir, 0o755); err != nil {
		return update.Release{}, err
	}
	if err := copyFile(o.Installer, filepath.Join(verDir, name)); err != nil {
		return update.Release{}, err
	}
	idx.Schema = update.SchemaVersion
	idx.Releases = append(idx.Releases, rel)
	slices.SortFunc(idx.Releases, func(a, b update.Release) int {
		c, _ := update.Compare(b.Version, a.Version) // já validadas
		return c
	})
	if err := writeJSON(filepath.Join(dir, "index.json"), idx); err != nil {
		return update.Release{}, err
	}
	if err := writeReleasesJS(dir, idx); err != nil {
		return update.Release{}, err
	}
	latest, err := marshal(update.Latest{Schema: update.SchemaVersion, Release: idx.Releases[0]})
	if err != nil {
		return update.Release{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), latest, 0o644); err != nil {
		return update.Release{}, err
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(o.Key, latest)) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "latest.json.sig"), []byte(sig), 0o644); err != nil {
		return update.Release{}, err
	}
	return rel, nil
}

// releasesDir é a pasta publicada. Fica na raiz do repositório do site porque
// a Hostinger publica o repositório inteiro, sem build: releases/ no repo é
// https://ae8.com.br/hyphp/releases/ no ar.
func releasesDir(web string) string { return filepath.Join(web, "releases") }

// releasesJSPrefix abre o releases.js. O site é HTML puro e precisa funcionar
// aberto direto do disco, onde o navegador bloqueia fetch() de arquivo local;
// um <script src> carrega de qualquer origem.
const releasesJSPrefix = "// Gerado por cmd/hyphp-release a partir de index.json. Não edite à mão.\nwindow.HYPHP_RELEASES = "

// writeReleasesJS grava o mesmo conteúdo do index.json como script.
func writeReleasesJS(dir string, idx update.Index) error {
	raw, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	out := append([]byte(releasesJSPrefix), raw...)
	out = append(out, ";\n"...)
	return os.WriteFile(filepath.Join(dir, "releases.js"), out, 0o644)
}

// Regenerate reescreve releases.js a partir do index.json existente, sem
// publicar nada: serve quando o arquivo some ou quando o formato do script muda.
func Regenerate(web string) error {
	dir := releasesDir(web)
	idx, err := readIndex(filepath.Join(dir, "index.json"))
	if err != nil {
		return err
	}
	if len(idx.Releases) == 0 {
		return fmt.Errorf("%s não tem versão publicada", filepath.Join(dir, "index.json"))
	}
	return writeReleasesJS(dir, idx)
}

func readIndex(path string) (update.Index, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return update.Index{Schema: update.SchemaVersion}, nil
	}
	if err != nil {
		return update.Index{}, err
	}
	var idx update.Index
	if err := json.Unmarshal(raw, &idx); err != nil {
		return update.Index{}, fmt.Errorf("%s ilegível: %w", path, err)
	}
	if idx.Schema != update.SchemaVersion {
		return update.Index{}, fmt.Errorf("%s: schema %d, esperado %d", path, idx.Schema, update.SchemaVersion)
	}
	return idx, nil
}

func marshal(v any) ([]byte, error) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func writeJSON(path string, v any) error {
	raw, err := marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func hashFile(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", fmt.Errorf("abrir instalador: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

var nshVersionRe = regexp.MustCompile(`!define INFO_PRODUCTVERSION "([^"]*)"`)

// checkVersions recusa publicar quando a versão compilada (version.Current)
// diverge da que vai no recurso do exe e no instalador. Um binário que se
// declara mais velho do que é acha sempre uma versão "nova" no manifesto e
// entra em loop de update. O wails_tools.nsh entra na conta porque é dele que
// sai a versão do instalador e o "DisplayVersion" em Aplicativos instalados,
// e nenhuma task o regenera no empacotamento.
func checkVersions(repo, current string) error {
	raw, err := os.ReadFile(filepath.Join(repo, "build", "config.yml"))
	if err != nil {
		return err
	}
	var cfg struct {
		Info struct {
			Version string `yaml:"version"`
		} `yaml:"info"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("build/config.yml: %w", err)
	}
	raw, err = os.ReadFile(filepath.Join(repo, "build", "windows", "info.json"))
	if err != nil {
		return err
	}
	var info struct {
		Fixed struct {
			FileVersion string `json:"file_version"`
		} `json:"fixed"`
		Info map[string]struct {
			ProductVersion string `json:"ProductVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return fmt.Errorf("build/windows/info.json: %w", err)
	}
	nsh, err := os.ReadFile(filepath.Join(repo, "build", "windows", "nsis", "wails_tools.nsh"))
	if err != nil {
		return err
	}
	nshVersion := ""
	if m := nshVersionRe.FindSubmatch(nsh); m != nil {
		nshVersion = string(m[1])
	}
	fontes := map[string]string{
		"build/config.yml info.version":                          cfg.Info.Version,
		"build/windows/info.json fixed.file_version":             info.Fixed.FileVersion,
		"build/windows/info.json ProductVersion":                 info.Info["0000"].ProductVersion,
		"build/windows/nsis/wails_tools.nsh INFO_PRODUCTVERSION": nshVersion,
	}
	for onde, v := range fontes {
		if v != current {
			return fmt.Errorf("versão divergente: internal/version.Current = %s, %s = %q", current, onde, v)
		}
	}
	return nil
}
