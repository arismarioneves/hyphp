package apache

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"hyphp/internal/runtime"
	"hyphp/internal/state"
	"hyphp/internal/supervisor"
	"hyphp/internal/webserver"
)

const (
	testApacheDir = "C:/hyphp/bin/apache/httpd-2.4.54-win64-VS16"
	testLogDir    = "C:/hyphp/log"
	// Nunca pode aparecer em arquivo gerado: é o etcDir de destino.
	testEtcDir = "C:/hyphp/etc/apache"
)

func testInstalled() runtime.Installed {
	return runtime.Installed{
		Kind:     runtime.Apache,
		Version:  "2.4.54",
		Major:    "2.4.54",
		Dir:      testApacheDir,
		Exe:      testApacheDir + "/bin/httpd.exe",
		Compiler: "VS16",
		Arch:     "x64",
	}
}

func testSites() []webserver.Site {
	return []webserver.Site{
		{
			ID:          "app81",
			Domain:      "app81.test",
			Aliases:     []string{"*.app81.test"},
			Docroot:     "C:/DEV/app81/public",
			PoolName:    "php81",
			TLSCert:     "C:/hyphp/var/certs/app81.pem",
			TLSKey:      "C:/hyphp/var/certs/app81-key.pem",
			HasHtaccess: true,
		},
		{
			ID:       "app72",
			Domain:   "app72.test",
			Docroot:  "C:/DEV/app72",
			PoolName: "php72",
		},
	}
}

func testPools() []webserver.PHPPool {
	return []webserver.PHPPool{
		{Name: "php81", Version: "8.1", Ports: []int{9000, 9001}},
		{Name: "php72", Version: "7.2", Ports: []int{9002, 9003}},
	}
}

var testPorts = webserver.Ports{HTTP: 8080, HTTPS: 8443}

func renderFixture(t *testing.T) map[string][]byte {
	t.Helper()
	files, err := New(testInstalled()).Render(testSites(), testPools(), testPorts, testLogDir, nil)
	if err != nil {
		t.Fatalf("Render erro = %v", err)
	}
	return files
}

func goldenName(key string) string {
	return strings.ReplaceAll(key, "/", "_") + ".golden"
}

func TestName(t *testing.T) {
	if got := New(testInstalled()).Name(); got != state.Apache {
		t.Fatalf("Name() = %q, quero %q", got, state.Apache)
	}
}

func TestRenderGolden(t *testing.T) {
	files := renderFixture(t)

	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"default/index.html", "httpd.conf", "pools.conf", "vhosts/app72.conf", "vhosts/app81.conf"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("chaves = %v, quero %v", keys, want)
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			path := filepath.Join("testdata", goldenName(key))
			wantBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ler golden: %v", err)
			}
			wantBytes = bytes.ReplaceAll(wantBytes, []byte("\r\n"), []byte("\n"))
			if !bytes.Equal(wantBytes, files[key]) {
				os.WriteFile(path+".got", files[key], 0o644)
				t.Fatalf("%s difere do golden; gravei %s.got", key, path)
			}
		})
	}
}

func TestRenderCorrecoesObrigatoriasDoWindows(t *testing.T) {
	files := renderFixture(t)

	const envIf = `ProxyFCGISetEnvIf "%{REQUEST_FILENAME} =~ m|([A-Za-z]:/.*)$|" SCRIPT_FILENAME "$1"`
	if !strings.Contains(string(files["httpd.conf"]), envIf) {
		t.Fatalf("httpd.conf não tem a linha exata do ProxyFCGISetEnvIf (spec §6.2):\n%s", files["httpd.conf"])
	}

	vhost := string(files["vhosts/app81.conf"])
	if !strings.Contains(vhost, `SetHandler "proxy:balancer://php81/"`) {
		t.Fatalf("SetHandler sem barra final devolve 400 Proxy Error (spec §6.2):\n%s", vhost)
	}
	if !strings.Contains(string(files["pools.conf"]), "ProxySet lbmethod=bybusyness") {
		t.Fatalf("pools.conf sem lbmethod=bybusyness:\n%s", files["pools.conf"])
	}
	if strings.Contains(string(files["pools.conf"]), "byrequests") {
		t.Fatal("byrequests serializa o pool: php-cgi atende um request por vez")
	}
}

func TestRenderNaoCarregaModuloProibido(t *testing.T) {
	proibidos := []string{"mod_fcgid", "mod_php", "LoadModule php"}
	for key, content := range renderFixture(t) {
		for _, p := range proibidos {
			if strings.Contains(string(content), p) {
				t.Errorf("%s referencia %q (spec §6.1: só módulos nativos de proxy)", key, p)
			}
		}
	}
}

func TestRenderEhRelocavel(t *testing.T) {
	files := renderFixture(t)
	for key, content := range files {
		if strings.Contains(string(content), testEtcDir) {
			t.Errorf("%s embute o etcDir %q; a validação em etc/apache.next leria a config antiga", key, testEtcDir)
		}
	}
	httpd := string(files["httpd.conf"])
	for _, want := range []string{
		`Include "${HYPHP_ETC}/pools.conf"`,
		`IncludeOptional "${HYPHP_ETC}/vhosts/*.conf"`,
		`DocumentRoot "${HYPHP_ETC}/default"`,
	} {
		if !strings.Contains(httpd, want) {
			t.Errorf("httpd.conf não contém %s", want)
		}
	}
}

func TestCommandDefineEtc(t *testing.T) {
	exe, args, dir := New(testInstalled()).Command(`C:\hyphp\etc\apache.next`)
	if exe != testApacheDir+"/bin/httpd.exe" {
		t.Fatalf("exe = %q", exe)
	}
	if dir != testApacheDir {
		t.Fatalf("dir = %q, quero o ServerRoot", dir)
	}
	want := []string{
		"-f", "C:/hyphp/etc/apache.next/httpd.conf",
		"-d", testApacheDir,
		"-C", "Define HYPHP_ETC C:/hyphp/etc/apache.next",
	}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %q, quero %q", args, want)
	}
	if slicesContains(args, "-t") {
		t.Fatal("Command não pode passar -t: -t só valida e sai")
	}
}

func slicesContains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestProbe(t *testing.T) {
	probe := New(testInstalled()).Probe(testPorts)
	http, ok := probe.(supervisor.HTTPProbe)
	if !ok {
		t.Fatalf("Probe() = %T, quero supervisor.HTTPProbe", probe)
	}
	if http.URL != "http://127.0.0.1:8080/" {
		t.Fatalf("Probe().URL = %q", http.URL)
	}
}
