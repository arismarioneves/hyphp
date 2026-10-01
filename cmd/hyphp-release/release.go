package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"debug/pe"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"unicode/utf16"

	"gopkg.in/yaml.v3"

	"hyphp/internal/update"
)

// Options descreve uma publicação.
type Options struct {
	WebDir    string // raiz do repositório do site
	Repo      string // "<dono>/<repo>" das releases no GitHub
	Installer string // instalador gerado por `wails3 task windows:package`
	Version   string
	Date      string   // AAAA-MM-DD
	Notes     []string // em português
	NotesEN   []string // em inglês, os mesmos itens na mesma ordem
	Key       ed25519.PrivateKey
	// GH roda o GitHub CLI com os argumentos dados; o teste troca por um
	// falso que só registra a chamada.
	GH func(args ...string) error
}

// Publish cria a release v<versão> no GitHub com três assets — o instalador,
// latest.json e latest.json.sig — e só então acrescenta a versão ao
// index.json e ao releases.js do site. Tudo o que pode recusar é checado
// antes do gh, e o site só muda depois de ele dar certo: uma publicação
// recusada não deixa rastro.
//
// A release é imutável depois de publicada (o repositório liga "immutable
// releases"): notas erradas no latest.json só se corrigem com versão nova.
func Publish(o Options) (update.Release, error) {
	dir := releasesDir(o.WebDir)
	idx, err := readIndex(filepath.Join(dir, "index.json"))
	if err != nil {
		return update.Release{}, err
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
	if err := checkNotes(o.Notes, o.NotesEN); err != nil {
		return update.Release{}, err
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
		NotesEN: append([]string{}, o.NotesEN...),
		// Só o nome: o app resolve o path contra a pasta do manifesto, que no
		// GitHub é releases/latest/download/.
		WindowsAMD64: &update.Artifact{Path: name, Size: size, SHA256: sum},
	}
	// A mesma validação que o app faz: nunca publicar o que ele recusaria.
	if err := rel.Validate(); err != nil {
		return update.Release{}, err
	}

	stage, err := os.MkdirTemp("", "hyphp-release-")
	if err != nil {
		return update.Release{}, err
	}
	defer os.RemoveAll(stage)
	assets, err := stageAssets(stage, o.Installer, name, rel, o.Key)
	if err != nil {
		return update.Release{}, err
	}
	notes := filepath.Join(stage, "notas.md")
	if err := os.WriteFile(notes, notesMarkdown(rel.Notes, rel.NotesEN), 0o644); err != nil {
		return update.Release{}, err
	}
	// --verify-tag: a tag tem de estar no GitHub antes, senão o gh criaria
	// uma apontando para o topo do main, que pode não ser o que foi compilado.
	args := []string{"release", "create", "v" + o.Version, "--repo", o.Repo, "--verify-tag", "--latest",
		"--title", "HyPHP " + o.Version, "--notes-file", notes}
	if err := o.GH(append(args, assets...)...); err != nil {
		return update.Release{}, fmt.Errorf("gh release create v%s: %w", o.Version, err)
	}

	idx.Schema = update.SchemaVersion
	idx.GitHub = o.Repo
	idx.Releases = append(idx.Releases, rel)
	slices.SortFunc(idx.Releases, func(a, b update.Release) int {
		c, _ := update.Compare(b.Version, a.Version) // já validadas
		return c
	})
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return update.Release{}, err
	}
	if err := writeJSON(filepath.Join(dir, "index.json"), idx); err != nil {
		return update.Release{}, err
	}
	return rel, writeReleasesJS(dir, idx)
}

// stageAssets monta em dir os três arquivos da release e devolve os caminhos
// na ordem em que vão para o gh. ed25519 assina os bytes exatos do
// latest.json, e é sobre esses bytes que o app roda Verify.
func stageAssets(dir, installer, name string, rel update.Release, key ed25519.PrivateKey) ([]string, error) {
	exe := filepath.Join(dir, name)
	if err := copyFile(installer, exe); err != nil {
		return nil, err
	}
	latest, err := marshal(update.Latest{Schema: update.SchemaVersion, Release: rel})
	if err != nil {
		return nil, err
	}
	latestPath := filepath.Join(dir, "latest.json")
	if err := os.WriteFile(latestPath, latest, 0o644); err != nil {
		return nil, err
	}
	sigPath := latestPath + ".sig"
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, latest)) + "\n"
	if err := os.WriteFile(sigPath, []byte(sig), 0o644); err != nil {
		return nil, err
	}
	return []string{exe, latestPath, sigPath}, nil
}

// checkNotes exige as notas nos dois idiomas, com o mesmo número de itens: o
// site mostra as do idioma escolhido, e a release é imutável, então uma
// tradução que faltar agora não entra depois. Sem nenhuma nota, o app
// mostraria a versão nova sem dizer o que mudou.
func checkNotes(pt, en []string) error {
	if len(pt) == 0 || len(en) == 0 {
		return errors.New("passe as notas nos dois idiomas: ao menos um -nota (português) e um -note (inglês)")
	}
	if len(pt) != len(en) {
		return fmt.Errorf("%d -nota e %d -note: cada nota precisa da tradução, na mesma ordem", len(pt), len(en))
	}
	return nil
}

// notesMarkdown vira a descrição da release: inglês primeiro, o idioma
// padrão do site, depois português, uma linha "- " por nota.
func notesMarkdown(pt, en []string) []byte {
	var b []byte
	b = append(b, "## English\n\n"...)
	for _, n := range en {
		b = append(b, "- "+n+"\n"...)
	}
	b = append(b, "\n## Português\n\n"...)
	for _, n := range pt {
		b = append(b, "- "+n+"\n"...)
	}
	return b
}

// runGH é o Options.GH de verdade: o gh herda o terminal, então login e
// progresso do upload aparecem para quem publica.
func runGH(args ...string) error {
	cmd := exec.Command("gh", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// releasesDir guarda o histórico que o site mostra. Fica na raiz do
// repositório do site porque a Hostinger publica o repositório inteiro, sem
// build. Os instaladores não moram mais aqui: ficam nas releases do GitHub.
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

// checkInstallerVersion recusa publicar um instalador que não seja da versão
// em publicação. checkVersions só olha os arquivos do repositório; se a versão
// subiu e o `wails3 task windows:package` não rodou de novo, o bin/ ainda tem
// o instalador anterior, e a release (imutável) entregaria o binário velho a
// todos os apps, que achariam de novo uma versão "nova": loop de update.
func checkInstallerVersion(path, want string) error {
	f, err := pe.Open(path)
	if err != nil {
		return fmt.Errorf("instalador %s não é um executável válido: %w", path, err)
	}
	defer f.Close()
	s := f.Section(".rsrc")
	if s == nil {
		return fmt.Errorf("instalador %s sem seção de recursos (.rsrc)", path)
	}
	rsrc, err := s.Data()
	if err != nil {
		return fmt.Errorf("instalador %s: ler recursos: %w", path, err)
	}
	if err := checkProductVersion(rsrc, want); err != nil {
		return fmt.Errorf("instalador %s: %w", path, err)
	}
	return nil
}

// checkProductVersion compara a string ProductVersion do recurso de versão com
// want, sem tolerar diferença: o NSIS grava ali o INFO_PRODUCTVERSION do
// wails_tools.nsh como está ("3.0.0", via VIAddVersionKey). O "3.0.0.0" de
// VIProductVersion vai só para o VS_FIXEDFILEINFO binário, que não é lido aqui.
func checkProductVersion(rsrc []byte, want string) error {
	got, err := productVersion(rsrc)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("ProductVersion = %q, mas internal/version.Current = %s: instalador desatualizado, rode `wails3 task windows:package` de novo", got, want)
	}
	return nil
}

// productVersionKey é o szKey da String procurada, em UTF-16LE com o NUL.
var productVersionKey = utf16LE("ProductVersion\x00")

// productVersion acha, nos bytes da seção .rsrc, a String "ProductVersion" da
// StringTable do VS_VERSIONINFO e devolve o valor. Procurar a chave evita
// percorrer a árvore de diretórios de recursos; o cabeçalho antes dela
// (wLength, wValueLength, wType = 1 de texto) confirma que é mesmo uma String
// e não a mesma sequência de bytes em outro recurso.
func productVersion(rsrc []byte) (string, error) {
	const hdr = 6 // wLength, wValueLength, wType
	for off := 0; ; {
		i := bytes.Index(rsrc[off:], productVersionKey)
		if i < 0 {
			return "", errors.New("recurso de versão sem ProductVersion")
		}
		i += off
		off = i + 1
		if i < hdr {
			continue
		}
		start := i - hdr
		end := start + int(binary.LittleEndian.Uint16(rsrc[start:]))
		if binary.LittleEndian.Uint16(rsrc[start+4:]) != 1 || end > len(rsrc) {
			continue
		}
		// O valor começa no próximo limite de 4 bytes depois da chave,
		// contado do início da String, e termina no NUL ou em wLength.
		v := start + (hdr+len(productVersionKey)+3)&^3
		var u []uint16
		for ; v+2 <= end; v += 2 {
			c := binary.LittleEndian.Uint16(rsrc[v:])
			if c == 0 {
				break
			}
			u = append(u, c)
		}
		return string(utf16.Decode(u)), nil
	}
}

func utf16LE(s string) []byte {
	var b []byte
	for _, c := range utf16.Encode([]rune(s)) {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	return b
}
