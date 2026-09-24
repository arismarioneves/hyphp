package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBuildTags(t *testing.T) {
	cases := []struct {
		name, in, compiler, arch string
	}{
		{"php vs16 x64", "php-8.1.10-Win32-vs16-x64", "vs16", "x64"},
		{"php VC15 x64", "php-7.2.34-Win32-VC15-x64", "VC15", "x64"},
		{"php nts x86", "php-8.3.33-nts-Win32-vs16-x86", "vs16", "x86"},
		{"apache lounge 2022", "httpd-2.4.54-win64-VS16", "VS16", "x64"},
		{"apache lounge 2026", "httpd-2.4.68-260920-Win64-VS18", "VS18", "x64"},
		{"banner httpd -v", "Server version: Apache/2.4.54 (Win64)\nApache Lounge VS16 Server built:   Jun 22 2022 09:58:15", "VS16", "x64"},
		{"nginx sem tags", "nginx-1.22.0", "", ""},
		{"mysql winx64 não é x64 nem win64", "mysql-8.0.30-winx64", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			compiler, arch := parseBuildTags(c.in)
			if compiler != c.compiler || arch != c.arch {
				t.Fatalf("parseBuildTags(%q) = (%q, %q); want (%q, %q)", c.in, compiler, arch, c.compiler, c.arch)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"8.1.10", "8.1.9", 1},
		{"8.1.9", "8.1.10", -1},
		{"8.1.10", "8.1.10", 0},
		{"8.4.0RC1", "8.3.33", 1},
		{"2.4.68", "2.4.68", 0},
		{"1.30", "1.30.0", 0},
	}
	for _, c := range cases {
		t.Run(c.a+"_vs_"+c.b, func(t *testing.T) {
			if got := compareVersions(c.a, c.b); got != c.want {
				t.Fatalf("compareVersions(%q,%q) = %d; want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestPHPByMajor(t *testing.T) {
	list := []Installed{
		{Kind: PHP, Version: "8.1.9", Major: "8.1"},
		{Kind: PHP, Version: "8.1.10", Major: "8.1"},
		{Kind: PHP, Version: "8.2.33", Major: "8.2"},
		{Kind: PHP, Version: "7.2.34", Major: "7.2"},
		{Kind: Apache, Version: "8.1.99", Major: "8.1.99"},
	}
	cases := []struct {
		major, wantVersion string
		wantOK             bool
	}{
		{"8.1", "8.1.10", true}, // numérico: 8.1.10 > 8.1.9 (lexicográfico erraria)
		{"8.2", "8.2.33", true},
		{"7.2", "7.2.34", true},
		{"8.4", "", false},
	}
	for _, c := range cases {
		t.Run(c.major, func(t *testing.T) {
			got, ok := PHPByMajor(list, c.major)
			if ok != c.wantOK || got.Version != c.wantVersion {
				t.Fatalf("PHPByMajor(%q) = (%q, %v); want (%q, %v)", c.major, got.Version, ok, c.wantVersion, c.wantOK)
			}
		})
	}
}

func TestByKind(t *testing.T) {
	list := []Installed{{Kind: PHP, Version: "8.1.10"}, {Kind: Nginx, Version: "1.22.0"}, {Kind: PHP, Version: "7.2.34"}}
	got := ByKind(list, PHP)
	if len(got) != 2 || got[0].Version != "8.1.10" || got[1].Version != "7.2.34" {
		t.Fatalf("ByKind(PHP) = %+v", got)
	}
	if got := ByKind(list, MySQL); len(got) != 0 {
		t.Fatalf("ByKind(MySQL) = %+v; want vazio", got)
	}
}

// Scan em um bin/ incompleto: pastas sem o executável esperado e kinds ausentes
// não podem gerar erro nem entradas fantasmas.
func TestScanIgnoresIncompleteDirs(t *testing.T) {
	bin := t.TempDir()
	mustMkdir(t, filepath.Join(bin, "php", "php-9.9.9-quebrado"))       // sem php.exe
	mustMkdir(t, filepath.Join(bin, "apache", "httpd-x", "bin"))        // sem httpd.exe
	mustMkdir(t, filepath.Join(bin, "mailpit"))                         // sem mailpit.exe
	mustWrite(t, filepath.Join(bin, "mysql", "arquivo-solto.txt"), "x") // arquivo em vez de pasta
	// bin/nginx e bin/mkcert não existem

	list, err := Scan(bin)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("Scan devolveu %d entradas em bin/ sem executáveis: %+v", len(list), list)
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
