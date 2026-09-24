// Package runtime descobre runtimes instalados em bin/ lendo a versão do próprio binário.
// A pasta é a fonte de verdade (spec §6.5): nada aqui consulta catálogo ou state.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Kind string

const (
	PHP     Kind = "php"
	Apache  Kind = "apache"
	Nginx   Kind = "nginx"
	MySQL   Kind = "mysql"
	Mailpit Kind = "mailpit"
	Mkcert  Kind = "mkcert"
)

type Installed struct {
	Kind       Kind   `json:"kind"`
	Version    string `json:"version"`    // "8.1.10", "2.4.62", "1.26.2", "8.0.30"
	Major      string `json:"major"`      // PHP: "8.1"; outros: igual a Version
	Dir        string `json:"dir"`        // bin/php/php-8.1.10-Win32-vs16-x64 (absoluto)
	Exe        string `json:"exe"`        // caminho absoluto do executável principal
	CGIExe     string `json:"cgiExe"`     // só PHP: php-cgi.exe
	Compiler   string `json:"compiler"`   // "vs16", "VC15", "VS17" ou ""
	Arch       string `json:"arch"`       // "x64" | "x86" | ""
	ThreadSafe *bool  `json:"threadSafe"` // só PHP
}

// detectTimeout limita cada execução de binário durante a detecção.
const detectTimeout = 5 * time.Second

// mainExe é o executável principal de cada Kind, relativo à pasta do runtime.
var mainExe = map[Kind]string{
	PHP:     "php.exe",
	Apache:  filepath.Join("bin", "httpd.exe"),
	Nginx:   "nginx.exe",
	MySQL:   filepath.Join("bin", "mysqld.exe"),
	Mailpit: "mailpit.exe",
	Mkcert:  "mkcert.exe",
}

// versioned marca os Kinds com uma subpasta por versão (bin/<kind>/<pasta>/).
// Mailpit e mkcert ficam direto em bin/<kind>/.
var versioned = map[Kind]bool{PHP: true, Apache: true, Nginx: true, MySQL: true}

// scanOrder fixa a ordem de saída de Scan.
var scanOrder = []Kind{PHP, Apache, Nginx, MySQL, Mailpit, Mkcert}

// Scan varre bin/<kind>/* e detecta cada runtime que tenha o executável esperado.
// Pastas sem o executável são ignoradas em silêncio. Falhas de detecção (exe presente
// mas não roda) são acumuladas no erro devolvido; a lista contém o que deu certo.
func Scan(binDir string) ([]Installed, error) {
	var list []Installed
	var errs []error
	for _, kind := range scanOrder {
		kindDir := filepath.Join(binDir, string(kind))
		var dirs []string
		if versioned[kind] {
			entries, err := os.ReadDir(kindDir)
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					errs = append(errs, fmt.Errorf("runtime: ler %s: %w", kindDir, err))
				}
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					dirs = append(dirs, filepath.Join(kindDir, e.Name()))
				}
			}
		} else {
			dirs = []string{kindDir}
		}
		for _, dir := range dirs {
			if _, err := os.Stat(filepath.Join(dir, mainExe[kind])); err != nil {
				continue
			}
			inst, err := Detect(kind, dir)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			list = append(list, inst)
		}
	}
	return list, errors.Join(errs...)
}

// Detect executa o binário principal de kind em dir e devolve o runtime descrito.
func Detect(kind Kind, dir string) (Installed, error) {
	rel, ok := mainExe[kind]
	if !ok {
		return Installed{}, fmt.Errorf("runtime: kind desconhecido %q", kind)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Installed{}, fmt.Errorf("runtime: %w", err)
	}
	exe := filepath.Join(abs, rel)
	if _, err := os.Stat(exe); err != nil {
		return Installed{}, fmt.Errorf("runtime: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), detectTimeout)
	defer cancel()
	switch kind {
	case PHP:
		return detectPHP(ctx, abs, exe)
	case Apache:
		return detectApache(ctx, abs, exe)
	case Nginx:
		return detectNginx(ctx, abs, exe)
	case MySQL:
		return detectMySQL(ctx, abs, exe)
	case Mailpit:
		return detectMailpit(ctx, abs, exe)
	default:
		return detectMkcert(ctx, abs, exe)
	}
}

// ByKind filtra list por k preservando a ordem.
func ByKind(list []Installed, k Kind) []Installed {
	var out []Installed
	for _, i := range list {
		if i.Kind == k {
			out = append(out, i)
		}
	}
	return out
}

// PHPByMajor devolve o PHP de maior patch da série major ("8.1").
func PHPByMajor(list []Installed, major string) (Installed, bool) {
	var best Installed
	found := false
	for _, i := range list {
		if i.Kind != PHP || i.Major != major {
			continue
		}
		if !found || compareVersions(i.Version, best.Version) > 0 {
			best, found = i, true
		}
	}
	return best, found
}

// run executa exe com args em dir, sem janela de console, e devolve stdout+stderr
// combinados (nginx -v escreve em stderr). ctx limita a duração.
func run(ctx context.Context, dir, exe string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("runtime: %s %s: %w: %s", filepath.Base(exe), strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

var (
	// versionRe acha "X.Y.Z" em qualquer posição.
	versionRe = regexp.MustCompile(`\d+\.\d+\.\d+`)
	// strictVersionRe exige que o texto inteiro seja uma versão ("8.4.0RC1" aceito).
	strictVersionRe = regexp.MustCompile(`^\d+\.\d+\.\d+\S*$`)
	compilerRe      = regexp.MustCompile(`(?i)\b(vc\d+|vs\d+)\b`)
	archRe          = regexp.MustCompile(`(?i)\b(x64|x86)\b`)
	winArchRe       = regexp.MustCompile(`(?i)\b(win64|win32)\b`)
)

// parseBuildTags extrai compilador (como escrito: "vs16", "VC15") e arquitetura
// normalizada ("x64"|"x86"|"") de um nome de pasta ou banner. Tokens x64/x86 têm
// prioridade sobre Win64/Win32 porque todo zip de PHP se chama "...-Win32-vs16-x64".
func parseBuildTags(s string) (compiler, arch string) {
	compiler = compilerRe.FindString(s)
	if m := archRe.FindString(s); m != "" {
		return compiler, strings.ToLower(m)
	}
	switch strings.ToLower(winArchRe.FindString(s)) {
	case "win64":
		arch = "x64"
	case "win32":
		arch = "x86"
	}
	return compiler, arch
}

// parseFirstVersion extrai a primeira "X.Y.Z" de out (com ou sem prefixo "v").
func parseFirstVersion(out, tool string) (string, error) {
	v := versionRe.FindString(out)
	if v == "" {
		return "", fmt.Errorf("runtime: saída inesperada do %s: %q", tool, strings.TrimSpace(out))
	}
	return v, nil
}

// majorOf devolve a série "major.minor" ("8.1.10" → "8.1").
func majorOf(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}

// compareVersions compara componente a componente numericamente; sufixos não
// numéricos ("RC1") são ignorados dentro do componente; componentes ausentes valem 0.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x = leadingInt(as[i])
		}
		if i < len(bs) {
			y = leadingInt(bs[i])
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}

func leadingInt(s string) int {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, _ := strconv.Atoi(s[:end])
	return n
}
