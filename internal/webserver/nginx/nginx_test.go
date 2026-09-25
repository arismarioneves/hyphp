package nginx

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
	testNginxDir = "C:/hyphp/bin/nginx/nginx-1.22.0"
	testLogDir   = "C:/hyphp/log"
	// Nunca pode aparecer em arquivo gerado: é o etcDir de destino.
	testEtcDir = "C:/hyphp/etc/nginx"
)

func testInstalled() runtime.Installed {
	return runtime.Installed{
		Kind:    runtime.Nginx,
		Version: "1.22.0",
		Major:   "1.22.0",
		Dir:     testNginxDir,
		Exe:     testNginxDir + "/nginx.exe",
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

func TestName(t *testing.T) {
	if got := New(testInstalled()).Name(); got != state.Nginx {
		t.Fatalf("Name() = %q, quero %q", got, state.Nginx)
	}
}

func TestRenderGolden(t *testing.T) {
	files := renderFixture(t)

	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{
		"html/index.html", "logs/.keep", "nginx.conf",
		"sites/app72.conf", "sites/app81.conf", "temp/.keep", "upstreams.conf",
	}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("chaves = %v, quero %v", keys, want)
	}
	if len(files["logs/.keep"]) != 0 || len(files["temp/.keep"]) != 0 {
		t.Fatal("os .keep têm de ser vazios: eles só declaram o diretório")
	}

	for _, key := range []string{"nginx.conf", "upstreams.conf", "sites/app81.conf", "sites/app72.conf", "html/index.html"} {
		t.Run(key, func(t *testing.T) {
			path := filepath.Join("testdata", strings.ReplaceAll(key, "/", "_")+".golden")
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

func TestRenderReceitaFastCGI(t *testing.T) {
	files := renderFixture(t)

	if !strings.Contains(string(files["upstreams.conf"]), "least_conn;") {
		t.Fatalf("upstreams.conf sem least_conn (equivalente do bybusyness):\n%s", files["upstreams.conf"])
	}
	site := string(files["sites/app81.conf"])
	if !strings.Contains(site, "fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;") {
		t.Fatalf("site sem SCRIPT_FILENAME montado a partir de $document_root:\n%s", site)
	}
	if !strings.Contains(site, "fastcgi_pass php81;") {
		t.Fatalf("site não aponta para o upstream php81:\n%s", site)
	}
	if !strings.Contains(site, "server_name app81.test *.app81.test;") {
		t.Fatalf("server_name não junta domínio e aliases:\n%s", site)
	}
	if !strings.Contains(site, "listen 8443 ssl;") {
		t.Fatalf("site com TLS não abriu o bloco HTTPS:\n%s", site)
	}
	if strings.Contains(string(files["sites/app72.conf"]), "ssl") {
		t.Fatalf("site sem TLS não pode ter bloco ssl:\n%s", files["sites/app72.conf"])
	}
}

func TestRenderEhRelocavel(t *testing.T) {
	files := renderFixture(t)
	for key, content := range files {
		if strings.Contains(string(content), testEtcDir) {
			t.Errorf("%s embute o etcDir %q; a validação em etc/nginx.next leria a config antiga", key, testEtcDir)
		}
	}
	conf := string(files["nginx.conf"])
	for _, want := range []string{"include upstreams.conf;", "include sites/*.conf;", "root html;"} {
		if !strings.Contains(conf, want) {
			t.Errorf("nginx.conf não contém %q (include relativo é o que torna -p suficiente)", want)
		}
	}
}

func TestCommandEValidateUsamPrefixo(t *testing.T) {
	exe, args, dir := New(testInstalled()).Command(`C:\hyphp\etc\nginx.next`)
	if exe != testNginxDir+"/nginx.exe" {
		t.Fatalf("exe = %q", exe)
	}
	if dir != testNginxDir {
		t.Fatalf("dir = %q, quero o diretório do nginx", dir)
	}
	want := []string{"-p", "C:/hyphp/etc/nginx.next/", "-c", "nginx.conf", "-g", "daemon off;"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %q, quero %q", args, want)
	}
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
