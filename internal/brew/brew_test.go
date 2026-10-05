package brew

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"hyphp/internal/pkgmgr"
	"hyphp/internal/runtime"
)

func TestTabelaDeFormulas(t *testing.T) {
	kinds := map[runtime.Kind]int{}
	names := map[string]bool{}
	for _, f := range Formulas {
		kinds[f.Kind]++
		if names[f.Name] {
			t.Errorf("fórmula repetida: %s", f.Name)
		}
		names[f.Name] = true
		if len(f.Opt) == 0 || f.Opt[0] != f.Short() {
			t.Errorf("%s: Opt = %v, a primeira pasta varrida deve ser o nome curto %q", f.Name, f.Opt, f.Short())
		}
		if f.Kind == runtime.PHP {
			if !strings.HasPrefix(f.Name, "shivammathur/php/") {
				t.Errorf("%s: PHP precisa do nome completo do tap (confia só na fórmula)", f.Name)
			}
			if want := strings.TrimPrefix(f.Short(), "php@"); f.Series != want {
				t.Errorf("%s: Series = %q, quer %q", f.Name, f.Series, want)
			}
		}
	}
	for _, k := range []runtime.Kind{runtime.PHP, runtime.Apache, runtime.Nginx, runtime.MySQL, runtime.MariaDB, runtime.Mailpit, runtime.Mkcert} {
		if kinds[k] == 0 {
			t.Errorf("nenhuma fórmula para %s", k)
		}
	}
	// phpMyAdmin continua vindo do catálogo, não do Homebrew.
	if kinds[runtime.PhpMyAdmin] != 0 {
		t.Error("phpMyAdmin não é fórmula do Homebrew no HyPHP")
	}
	for _, proibido := range []string{"mysql@8.0", "mysql", "mariadb"} {
		if names[proibido] {
			t.Errorf("%s está fora do escopo (spec) e não pode estar na tabela", proibido)
		}
	}
	var php85 *Formula
	for i := range Formulas {
		if Formulas[i].Name == "shivammathur/php/php@8.5" {
			php85 = &Formulas[i]
		}
	}
	if php85 == nil {
		t.Fatal("falta shivammathur/php/php@8.5")
	}
	if want := []string{"php@8.5", "php"}; !reflect.DeepEqual(php85.Opt, want) {
		t.Errorf("8.5 varre %v, quer %v", php85.Opt, want)
	}
}

// symlinkOuPula cria o link ou pula o teste quando o SO exige privilégio
// (Windows sem modo desenvolvedor); no job do macOS o teste roda de verdade.
func symlinkOuPula(t *testing.T, alvo, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(alvo, link); err != nil {
		if errors.Is(err, os.ErrPermission) || strings.Contains(err.Error(), "privilege") {
			t.Skipf("symlink exige privilégio aqui: %v", err)
		}
		t.Fatal(err)
	}
}

func TestScanNoPrefixoFalso(t *testing.T) {
	prefix := t.TempDir()
	keg := func(nome, versao string) string {
		d := filepath.Join(prefix, "Cellar", nome, versao)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		return d
	}
	opt := func(nome string) string { return filepath.Join(prefix, "opt", nome) }

	symlinkOuPula(t, keg("php@8.3", "8.3.20"), opt("php@8.3"))
	php := keg("php", "8.5.1")
	symlinkOuPula(t, php, opt("php"))
	symlinkOuPula(t, php, opt("php@8.5"))
	symlinkOuPula(t, keg("httpd", "2.4.65"), opt("httpd"))
	// nginx sem binário que rode: a detecção falha e a pasta não entra.
	if err := os.MkdirAll(opt("nginx"), 0o755); err != nil {
		t.Fatal(err)
	}

	versoes := map[string]string{"php@8.3": "8.3.20", "php@8.5": "8.5.1", "php": "8.5.1", "httpd": "2.4.65"}
	orig := detect
	t.Cleanup(func() { detect = orig })
	var vistos []string
	detect = func(kind runtime.Kind, dir string) (runtime.Installed, error) {
		vistos = append(vistos, dir)
		v, ok := versoes[filepath.Base(dir)]
		if !ok {
			return runtime.Installed{}, fmt.Errorf("runtime: %s não roda", dir)
		}
		return runtime.Installed{Kind: kind, Version: v, Major: runtime.MajorOf(v), Dir: dir}, nil
	}

	got, err := Brew{Exe: filepath.Join(prefix, "bin", "brew"), Prefix: prefix}.Scan(context.Background())
	if err == nil || !strings.Contains(err.Error(), "nginx") {
		t.Errorf("erro = %v, quer a falha de detecção do nginx", err)
	}
	for _, d := range vistos {
		if strings.Contains(d, "Cellar") {
			t.Errorf("detect recebeu o caminho do Cellar %s; deve receber o opt/", d)
		}
	}
	want := []runtime.Installed{
		{Kind: runtime.PHP, Version: "8.5.1", Major: "8.5", Dir: opt("php@8.5"), Formula: "shivammathur/php/php@8.5", Prefix: prefix},
		{Kind: runtime.PHP, Version: "8.3.20", Major: "8.3", Dir: opt("php@8.3"), Formula: "shivammathur/php/php@8.3", Prefix: prefix},
		{Kind: runtime.Apache, Version: "2.4.65", Major: "2.4.65", Dir: opt("httpd"), Formula: "httpd", Prefix: prefix},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Scan =\n%+v\nquer\n%+v", got, want)
	}
}

func TestPhaseOf(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "brew_install_php.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var got []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if p := phaseOf(sc.Text()); p != "" {
			got = append(got, p)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	d, x := pkgmgr.PhaseDownload, pkgmgr.PhaseExtract
	// Fetching, Downloading, Fetching deps, Downloading ×2, Installing deps,
	// Installing dependency, Pouring, Installing, Pouring.
	want := []string{d, d, d, d, d, x, x, x, x, x}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fases = %v, quer %v", got, want)
	}
	for _, l := range []string{
		"==> Caveats",
		"==> Summary",
		"🍺  /opt/homebrew/Cellar/php@8.3/8.3.20: 520 files, 85MB",
		"==> /opt/homebrew/Cellar/php@8.3/8.3.20/bin/pear config-set php_ini /opt/homebrew/etc/php/8.3/php.ini system",
		"Already downloaded: /Users/dev/Library/Caches/Homebrew/downloads/x.json",
		"",
	} {
		if p := phaseOf(l); p != "" {
			t.Errorf("phaseOf(%q) = %q, quer \"\"", l, p)
		}
	}
}
