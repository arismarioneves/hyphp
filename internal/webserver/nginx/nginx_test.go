package nginx

import (
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

// Mesmo motivo do apache: "include sites/*.conf" falha se o diretório não
// existir, e sem projeto nenhum ele não existiria.
func TestRenderSemProjetosMantemDiretorioSites(t *testing.T) {
	files, err := New(testInstalled()).Render(nil, nil, testPorts, testLogDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files["sites/.dir"]; !ok {
		t.Error("falta sites/.dir; o nginx recusaria a config")
	}
}

// O nginx no Windows usa 32 como server_names_hash_bucket_size, e um nome com
// mais de ~22 caracteres já não cabe: "could not build server_names_hash". A
// regra do ngx_hash_init é bucket >= alinha8(len+2) + 16 (ponteiro de 8 bytes,
// que cobre as builds de 32 e 64 bits); 46 caracteres é o máximo em 64.
func TestRenderBucketCabeONomeMaisLongo(t *testing.T) {
	nome := func(n int) string { return strings.Repeat("a", n-len(".test")) + ".test" }
	casos := []struct {
		nome   string
		sites  []webserver.Site
		tool   *webserver.Tool
		bucket string
	}{
		{"nomes curtos", testSites(), nil, "64"},
		{"46 caracteres ainda cabem em 64", []webserver.Site{{ID: "x", Domain: nome(46)}}, nil, "64"},
		{"47 caracteres pedem 128", []webserver.Site{{ID: "x", Domain: nome(47)}}, nil, "128"},
		{"alias conta", []webserver.Site{{ID: "x", Domain: nome(45), Aliases: []string{"*." + nome(45)}}}, nil, "128"},
		{"ferramenta conta", nil, &webserver.Tool{Name: strings.Repeat("f", 41), Port: 8036}, "128"},
		{"nome de 253 caracteres, o máximo do DNS", []webserver.Site{{ID: "x", Domain: nome(253)}}, nil, "512"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			files, err := New(testInstalled()).Render(c.sites, testPools(), testPorts, testLogDir, c.tool)
			if err != nil {
				t.Fatal(err)
			}
			want := "server_names_hash_bucket_size " + c.bucket + ";"
			if !strings.Contains(string(files["nginx.conf"]), want) {
				t.Fatalf("nginx.conf sem %q:\n%s", want, files["nginx.conf"])
			}
		})
	}
}
