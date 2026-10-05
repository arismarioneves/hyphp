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
			if got := CompareVersions(c.a, c.b); got != c.want {
				t.Fatalf("CompareVersions(%q,%q) = %d; want %d", c.a, c.b, got, c.want)
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

// O 4º campo é basename(PHP_EXTENSION_DIR): "ext" no zip do Windows, o número
// da API ("20230831") no Homebrew.
func TestParsePHPProbe(t *testing.T) {
	cases := []struct {
		name, out, version string
		intSize            int
		zts                bool
		apiBase            string
		wantErr            bool
	}{
		{"8.1.10 zts x64 (laragon)", "8.1.10|8|zts|ext", "8.1.10", 8, true, "ext", false},
		{"7.2.34 zts x64 (laragon)", "7.2.34|8|zts|ext", "7.2.34", 8, true, "ext", false},
		{"nts x86 com CRLF", "8.3.33|4|nts|ext\r\n", "8.3.33", 4, false, "ext", false},
		{"release candidate", "8.4.0RC1|8|nts|ext", "8.4.0RC1", 8, false, "ext", false},
		{"homebrew arm64", "8.3.20|8|nts|20230831", "8.3.20", 8, false, "20230831", false},
		{"warning de php.ini antes da saída", "Warning: PHP Startup: Unable to load dynamic library 'curl'\n8.1.10|8|zts|ext", "", 0, false, "", true},
		{"vazio", "", "", 0, false, "", true},
		{"campo faltando", "8.1.10|8|zts", "", 0, false, "", true},
		{"pasta de módulos vazia", "8.1.10|8|zts|", "", 0, false, "", true},
		{"thread-safety inválida", "8.1.10|8|tsrm|ext", "", 0, false, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			version, intSize, zts, apiBase, err := parsePHPProbe(c.out)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v; wantErr %v", err, c.wantErr)
			}
			if version != c.version || intSize != c.intSize || zts != c.zts || apiBase != c.apiBase {
				t.Fatalf("got (%q, %d, %v, %q); want (%q, %d, %v, %q)", version, intSize, zts, apiBase, c.version, c.intSize, c.zts, c.apiBase)
			}
		})
	}
}

func TestParseApacheVersion(t *testing.T) {
	cases := []struct {
		name, out, version, compiler, arch string
		wantErr                            bool
	}{
		{
			"apache lounge 2.4.54 VS16 (laragon)",
			"Server version: Apache/2.4.54 (Win64)\r\nApache Lounge VS16 Server built:   Jun 22 2022 09:58:15\r\n",
			"2.4.54", "VS16", "x64", false,
		},
		{
			"apache lounge 2.4.68 VS18 win32",
			"Server version: Apache/2.4.68 (Win32)\nApache Lounge VS18 Server built:   Sep 20 2026 10:00:00\n",
			"2.4.68", "VS18", "x86", false,
		},
		{"sem banner", "httpd: illegal option -- v", "", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			version, compiler, arch, err := parseApacheVersion(c.out)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v; wantErr %v", err, c.wantErr)
			}
			if version != c.version || compiler != c.compiler || arch != c.arch {
				t.Fatalf("got (%q, %q, %q); want (%q, %q, %q)", version, compiler, arch, c.version, c.compiler, c.arch)
			}
		})
	}
}

func TestParseNginxVersion(t *testing.T) {
	cases := []struct {
		name, out, want string
		wantErr         bool
	}{
		{"1.22.0 (laragon, stderr)", "nginx version: nginx/1.22.0\r\n", "1.22.0", false},
		{"1.30.5", "nginx version: nginx/1.30.5", "1.30.5", false},
		{"vazio", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseNginxVersion(c.out)
			if (err != nil) != c.wantErr || got != c.want {
				t.Fatalf("parseNginxVersion(%q) = (%q, %v); want (%q, wantErr=%v)", c.out, got, err, c.want, c.wantErr)
			}
		})
	}
}

func TestParseMySQLVersion(t *testing.T) {
	cases := []struct {
		name, out, version, arch string
		wantErr                  bool
	}{
		{
			"8.0.30 (laragon)",
			"C:\\laragon\\bin\\mysql\\mysql-8.0.30-winx64\\bin\\mysqld.exe  Ver 8.0.30 for Win64 on x86_64 (MySQL Community Server - GPL)\r\n",
			"8.0.30", "x64", false,
		},
		{
			"8.4.11 caminho com barras normais",
			"C:/hyphp/.runtime/bin/mysql/mysql-8.4.11-winx64/bin/mysqld.exe  Ver 8.4.11 for Win64 on x86_64 (MySQL Community Server - GPL)",
			"8.4.11", "x64", false,
		},
		{"sem Ver", "mysqld: unknown option '--versionx'", "", "", true},
		// O zip do MariaDB traz um mysqld.exe que responde assim.
		{"mysqld do MariaDB", "C:/x/mariadb-10.11.19-winx64/bin/mysqld.exe  Ver 10.11.19-MariaDB for Win64 on AMD64 (MariaDB Server)", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			version, arch, err := parseMySQLVersion(c.out)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v; wantErr %v", err, c.wantErr)
			}
			if version != c.version || arch != c.arch {
				t.Fatalf("got (%q, %q); want (%q, %q)", version, arch, c.version, c.arch)
			}
		})
	}
}

// Saídas reais do mariadbd.exe --no-defaults --version (10.11.19 e 12.3.3).
func TestParseMariaDBVersion(t *testing.T) {
	cases := []struct {
		name, out, version, arch string
		wantErr                  bool
	}{
		{"10.11", "C:/DEV/.mariadb/mariadb-10.11.19-winx64/bin/mariadbd.exe  Ver 10.11.19-MariaDB for Win64 on AMD64 (MariaDB Server)", "10.11.19", "x64", false},
		{"12.3", "C:/DEV/.mariadb/mariadb-12.3.3-winx64/bin/mariadbd.exe  Ver 12.3.3-MariaDB for Win64 on AMD64 (MariaDB Server)\r\n", "12.3.3", "x64", false},
		{"mysqld do MySQL não é MariaDB", "mysqld.exe  Ver 8.4.11 for Win64 on x86_64 (MySQL Community Server - GPL)", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			version, arch, err := parseMariaDBVersion(c.out)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v; wantErr %v", err, c.wantErr)
			}
			if version != c.version || arch != c.arch {
				t.Fatalf("got (%q, %q); want (%q, %q)", version, arch, c.version, c.arch)
			}
		})
	}
}

func TestParseFirstVersion(t *testing.T) {
	cases := []struct {
		name, out, want string
		wantErr         bool
	}{
		{"mailpit com v", "v1.21.5\n", "1.21.5", false},
		{"mailpit sem v", "1.21.5", "1.21.5", false},
		{"mkcert", "v1.4.4\r\n", "1.4.4", false},
		{"texto ao redor", "Mailpit v1.31.1 compiled with Go 1.25.1", "1.31.1", false},
		{"sem versão", "usage: mailpit [flags]", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseFirstVersion(c.out, "tool")
			if (err != nil) != c.wantErr || got != c.want {
				t.Fatalf("parseFirstVersion(%q) = (%q, %v); want (%q, wantErr=%v)", c.out, got, err, c.want, c.wantErr)
			}
		})
	}
}
