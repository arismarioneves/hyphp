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
	"strings"
	"unicode/utf16"

	"gopkg.in/yaml.v3"

	"hyphp/internal/update"
)

// Files são os arquivos que os jobs windows e macos do build.yml entregam.
type Files struct {
	Installer string // instalador NSIS (bin/hyphp-amd64-installer.exe)
	DMG       string // dmg do Mac (bin/HyPHP.dmg)
	Install   string // build/darwin/install.sh
	Uninstall string // build/darwin/uninstall.sh
}

// Notes é o release/notas/<versão>.json: as mesmas notas em português e em
// inglês, na mesma ordem.
type Notes struct {
	PT []string `json:"pt"`
	EN []string `json:"en"`
}

// Options descreve uma publicação.
type Options struct {
	WebDir  string // checkout do repositório do site
	Repo    string // "<dono>/<repo>" das releases no GitHub
	Version string
	Date    string // AAAA-MM-DD
	Notes   Notes
	Files   Files
	Key     ed25519.PrivateKey
	// GH roda o GitHub CLI com os argumentos dados e devolve o stdout; o teste
	// troca por um falso que registra a chamada e responde como o GitHub.
	GH func(args ...string) ([]byte, error)
}

// Plataformas do manifesto, com os nomes dos campos do latest.json.
const (
	windowsAMD64 = "windows_amd64"
	darwinARM64  = "darwin_arm64"
)

// assetName é o nome do instalador de cada plataforma na release. É também o
// path no latest.json: o app resolve o path contra a pasta do manifesto, que
// no GitHub é releases/latest/download/.
func assetName(version, platform string) string {
	switch platform {
	case windowsAMD64:
		return "hyphp-" + version + "-windows-amd64-setup.exe"
	case darwinARM64:
		return "hyphp-" + version + "-darwin-arm64.dmg"
	}
	panic("assetName: plataforma desconhecida " + platform)
}

// Publish cria a release v<versão> no GitHub com seis assets — os dois
// instaladores, install.sh, uninstall.sh, latest.json e latest.json.sig — e
// só então acrescenta a versão ao index.json e ao releases.js do site.
//
// A release nasce em draft, invisível: os digests que o GitHub calculou são
// conferidos contra os arquivos enviados antes de ela virar pública e Latest.
// Tudo o que pode recusar é checado antes do create, e o site só muda depois
// da publicação: uma publicação recusada deixa no máximo um draft, e a
// reexecução recusa enquanto ele existir.
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
	if err := checkNotes(o.Notes); err != nil {
		return update.Release{}, err
	}
	rel, err := buildRelease(o.Version, o.Date, o.Notes, o.Files)
	if err != nil {
		return update.Release{}, err
	}

	stage, err := os.MkdirTemp("", "hyphp-release-")
	if err != nil {
		return update.Release{}, err
	}
	defer os.RemoveAll(stage)
	assets, err := stageAssets(stage, o.Files, rel, o.Key)
	if err != nil {
		return update.Release{}, err
	}
	notes := filepath.Join(stage, "notas.md")
	if err := os.WriteFile(notes, notesMarkdown(rel.Notes, rel.NotesEN), 0o644); err != nil {
		return update.Release{}, err
	}
	tag := "v" + o.Version
	if err := checkNoRelease(o.GH, o.Repo, tag); err != nil {
		return update.Release{}, err
	}
	// --verify-tag: a tag tem de estar no GitHub antes, senão o gh criaria
	// uma apontando para o topo do main, que pode não ser o que foi compilado.
	args := []string{"release", "create", tag, "--repo", o.Repo, "--draft", "--verify-tag",
		"--title", "HyPHP " + o.Version, "--notes-file", notes}
	if _, err := o.GH(append(args, assets...)...); err != nil {
		return update.Release{}, fmt.Errorf("gh release create %s: %w", tag, err)
	}
	view, err := o.GH("release", "view", tag, "--repo", o.Repo, "--json", "assets")
	if err != nil {
		return update.Release{}, fmt.Errorf("gh release view %s: %w (a release ficou em draft)", tag, err)
	}
	if err := checkDigests(view, assets, rel); err != nil {
		return update.Release{}, fmt.Errorf("%w (a release %s ficou em draft)", err, tag)
	}
	if _, err := o.GH("release", "edit", tag, "--repo", o.Repo, "--draft=false", "--latest"); err != nil {
		return update.Release{}, fmt.Errorf("gh release edit %s: %w (a release ficou em draft)", tag, err)
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

// checkNoRelease recusa a publicação se o GitHub já tem release com a tag. Um
// draft sobra de uma publicação que parou depois do create; criar outro
// deixaria dois drafts com o mesmo tag_name, e o view/edit por tag resolveria
// um deles sem ordem garantida. Nada é apagado aqui: quem reexecuta decide.
func checkNoRelease(gh func(args ...string) ([]byte, error), repo, tag string) error {
	raw, err := gh("release", "list", "--repo", repo, "--json", "tagName,isDraft", "--limit", "100")
	if err != nil {
		return fmt.Errorf("gh release list: %w", err)
	}
	var list []struct {
		TagName string `json:"tagName"`
		IsDraft bool   `json:"isDraft"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return fmt.Errorf("gh release list ilegível: %w", err)
	}
	for _, r := range list {
		if r.TagName != tag {
			continue
		}
		if r.IsDraft {
			return fmt.Errorf("já existe um draft da release %s, de uma publicação anterior; apague-o com `gh release delete %s --repo %s --yes` e reexecute", tag, tag, repo)
		}
		return fmt.Errorf("a release %s já está publicada e é imutável", tag)
	}
	return nil
}

// buildRelease monta o Release com os dois instaladores, passando pela mesma
// validação que o app faz: nunca publicar o que ele recusaria.
func buildRelease(version, date string, notes Notes, f Files) (update.Release, error) {
	rel := update.Release{Version: version, Date: date, Notes: slices.Clone(notes.PT), NotesEN: slices.Clone(notes.EN)}
	for _, a := range []struct {
		file, platform string
		dst            **update.Artifact
	}{
		{f.Installer, windowsAMD64, &rel.WindowsAMD64},
		{f.DMG, darwinARM64, &rel.DarwinARM64},
	} {
		size, sum, err := hashFile(a.file)
		if err != nil {
			return update.Release{}, err
		}
		*a.dst = &update.Artifact{Path: assetName(version, a.platform), Size: size, SHA256: sum}
	}
	if err := rel.Validate(); err != nil {
		return update.Release{}, err
	}
	return rel, nil
}

// signLatest serializa o latest.json e o assina. ed25519 assina os bytes
// exatos, e é sobre esses bytes que o app roda Verify.
func signLatest(rel update.Release, key ed25519.PrivateKey) (latest, sig []byte, err error) {
	latest, err = marshal(update.Latest{Schema: update.SchemaVersion, Release: rel})
	if err != nil {
		return nil, nil, err
	}
	sig = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, latest)) + "\n")
	return latest, sig, nil
}

// stageAssets monta em dir os seis arquivos da release com os nomes que ficam
// no GitHub e devolve os caminhos na ordem em que vão para o gh.
func stageAssets(dir string, f Files, rel update.Release, key ed25519.PrivateKey) ([]string, error) {
	var out []string
	for _, c := range []struct{ src, name string }{
		{f.Installer, rel.WindowsAMD64.Path},
		{f.DMG, rel.DarwinARM64.Path},
		{f.Install, "install.sh"},
		{f.Uninstall, "uninstall.sh"},
	} {
		dst := filepath.Join(dir, c.name)
		if err := copyFile(c.src, dst); err != nil {
			return nil, err
		}
		out = append(out, dst)
	}
	latest, sig, err := signLatest(rel, key)
	if err != nil {
		return nil, err
	}
	latestPath := filepath.Join(dir, "latest.json")
	if err := os.WriteFile(latestPath, latest, 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(latestPath+".sig", sig, 0o644); err != nil {
		return nil, err
	}
	return append(out, latestPath, latestPath+".sig"), nil
}

// checkDigests compara o digest que o GitHub calculou de cada asset com o
// sha256 do arquivo enviado. Os dois instaladores têm o sha256 também no
// latest.json, e o app confere contra ele: se o GitHub guardou outros bytes,
// todo app recusaria o update. A release ainda é draft aqui, invisível.
func checkDigests(view []byte, assets []string, rel update.Release) error {
	var v struct {
		Assets []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(view, &v); err != nil {
		return fmt.Errorf("gh release view ilegível: %w", err)
	}
	got := map[string]string{}
	for _, a := range v.Assets {
		got[a.Name] = a.Digest
	}
	inLatest := map[string]string{
		rel.WindowsAMD64.Path: rel.WindowsAMD64.SHA256,
		rel.DarwinARM64.Path:  rel.DarwinARM64.SHA256,
	}
	for _, p := range assets {
		name := filepath.Base(p)
		_, sum, err := hashFile(p)
		if err != nil {
			return err
		}
		if want, ok := inLatest[name]; ok && want != sum {
			return fmt.Errorf("%s: sha256 %s diverge do latest.json (%s)", name, sum, want)
		}
		if got[name] != "sha256:"+sum {
			return fmt.Errorf("%s: o GitHub guardou o digest %q, e o arquivo enviado tem sha256:%s", name, got[name], sum)
		}
	}
	return nil
}

// checkNotes exige as notas nos dois idiomas, com o mesmo número de itens: o
// site mostra as do idioma escolhido, e a release é imutável, então uma
// tradução que faltar agora não entra depois. Sem nenhuma nota, o app
// mostraria a versão nova sem dizer o que mudou.
func checkNotes(n Notes) error {
	if len(n.PT) == 0 || len(n.EN) == 0 {
		return errors.New(`as notas precisam de ao menos um item em "pt" e um em "en"`)
	}
	if len(n.PT) != len(n.EN) {
		return fmt.Errorf("%d notas em pt e %d em en: cada nota precisa da tradução, na mesma ordem", len(n.PT), len(n.EN))
	}
	return nil
}

// notesPath é o arquivo de notas de cada versão, criado e revisado no PR que
// sobe a versão.
func notesPath(repo, version string) string {
	return filepath.Join(repo, "release", "notas", version+".json")
}

// readNotes lê e valida o arquivo de notas. Chave desconhecida é erro: um
// "pt-BR" no lugar de "pt" deixaria o português de fora sem aviso.
func readNotes(repo, version string) (Notes, error) {
	p := notesPath(repo, version)
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return Notes{}, fmt.Errorf("%s ausente: crie o arquivo de notas no PR que sobe a versão", p)
	}
	if err != nil {
		return Notes{}, err
	}
	var n Notes
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&n); err != nil {
		return Notes{}, fmt.Errorf("%s ilegível: %w", p, err)
	}
	if err := checkNotes(n); err != nil {
		return Notes{}, fmt.Errorf("%s: %w", p, err)
	}
	return n, nil
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

// verify é o modo -verificar, o primeiro job do build.yml, antes de compilar.
// Sem tag (o ensaio) confere só os arquivos de versão entre si; com a tag,
// também que ela é a versão compilada, que as notas existem e que a versão é
// maior que a da última release publicada.
func verify(repo, current, tag, ghRepo string, gh func(args ...string) ([]byte, error)) error {
	if err := checkVersions(repo, current); err != nil {
		return err
	}
	if tag == "" {
		return nil
	}
	if tag != "v"+current {
		return fmt.Errorf("a tag %s não é a versão compilada (internal/version.Current = %s)", tag, current)
	}
	if _, err := readNotes(repo, current); err != nil {
		return err
	}
	raw, err := gh("release", "view", "--repo", ghRepo, "--json", "tagName")
	if err != nil {
		return fmt.Errorf("gh release view: %w", err)
	}
	var last struct {
		TagName string `json:"tagName"`
	}
	if err := json.Unmarshal(raw, &last); err != nil {
		return fmt.Errorf("gh release view ilegível: %w", err)
	}
	c, err := update.Compare(current, strings.TrimPrefix(last.TagName, "v"))
	if err != nil {
		return err
	}
	if c <= 0 {
		return fmt.Errorf("versão %s não é maior que a da última release publicada (%s)", current, last.TagName)
	}
	return nil
}

// releaseKeyEnv guarda a seed ed25519 em hex que assina o latest.json.
const releaseKeyEnv = "HYPHP_RELEASE_KEY"

// releaseKey decodifica a seed e exige que ela corresponda a pub. Assinar com
// uma chave que o binário não conhece publicaria um manifesto que todo app
// instalado recusa — em silêncio, do lado do usuário.
func releaseKey(seedHex string, pub ed25519.PublicKey) (ed25519.PrivateKey, error) {
	seed, err := hex.DecodeString(strings.TrimSpace(seedHex))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("%s ausente ou sem uma seed ed25519 em hex", releaseKeyEnv)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	if !priv.Public().(ed25519.PublicKey).Equal(pub) {
		return nil, fmt.Errorf("a chave de %s não corresponde a update.PublicKey embutida no app", releaseKeyEnv)
	}
	return priv, nil
}

// rehearse é o modo -ensaio: monta e assina o latest.json da versão em
// memória e confere a assinatura com a chave pública do app, como ele faria.
// Nada é gravado nem publicado; as notas ficam vazias, o ensaio não tem tag.
func rehearse(version, date string, f Files, key ed25519.PrivateKey, pub ed25519.PublicKey) (update.Latest, error) {
	rel, err := buildRelease(version, date, Notes{}, f)
	if err != nil {
		return update.Latest{}, err
	}
	latest, sig, err := signLatest(rel, key)
	if err != nil {
		return update.Latest{}, err
	}
	if err := update.Verify(pub, latest, sig); err != nil {
		return update.Latest{}, err
	}
	return update.ParseLatest(latest)
}

// runGH é o Options.GH de verdade: o stdout volta para quem chamou (o view é
// JSON), e o stderr do gh, com erros e progresso, vai para o log do job.
func runGH(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

// releasesDir guarda o histórico que o site mostra. Fica na raiz do
// repositório do site, que é publicado como está, sem build. Os instaladores
// não moram mais aqui: ficam nas releases do GitHub.
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
		return 0, "", fmt.Errorf("abrir %s: %w", path, err)
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

// plistString extrai o <string> que segue uma <key> do Info.plist; o arquivo
// é escrito à mão e tem só strings nessas chaves, então regex basta.
func plistString(plist []byte, chave string) string {
	re := regexp.MustCompile(`<key>` + regexp.QuoteMeta(chave) + `</key>\s*<string>([^<]*)</string>`)
	if m := re.FindSubmatch(plist); m != nil {
		return string(m[1])
	}
	return ""
}

// checkVersions recusa publicar quando a versão compilada (version.Current)
// diverge da que vai no recurso do exe e no instalador. Um binário que se
// declara mais velho do que é acha sempre uma versão "nova" no manifesto e
// entra em loop de update. O wails_tools.nsh entra na conta porque é dele que
// sai a versão do instalador e o "DisplayVersion" em Aplicativos instalados,
// e nenhuma task o regenera no empacotamento. O build/darwin/Info.plist também:
// o macOS mostra essa versão em "Sobre" e o update da M3 compara com ela.
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
	plist, err := os.ReadFile(filepath.Join(repo, "build", "darwin", "Info.plist"))
	if err != nil {
		return err
	}
	fontes := map[string]string{
		"build/config.yml info.version":                          cfg.Info.Version,
		"build/windows/info.json fixed.file_version":             info.Fixed.FileVersion,
		"build/windows/info.json ProductVersion":                 info.Info["0000"].ProductVersion,
		"build/windows/nsis/wails_tools.nsh INFO_PRODUCTVERSION": nshVersion,
		"build/darwin/Info.plist CFBundleShortVersionString":     plistString(plist, "CFBundleShortVersionString"),
		"build/darwin/Info.plist CFBundleVersion":                plistString(plist, "CFBundleVersion"),
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
