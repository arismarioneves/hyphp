package apache

import (
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

// Sem nenhum projeto o diretório vhosts/ precisa existir mesmo assim.
// IncludeOptional cobre arquivo ausente, não diretório ausente: sem isso o
// httpd -t recusa a config inteira, o Reconcile falha no boot e NENHUM serviço
// é criado — nem o MySQL, nem o Mailpit, que não têm relação com vhost.
func TestRenderSemProjetosMantemDiretoriosDeInclude(t *testing.T) {
	files, err := New(testInstalled()).Render(nil, nil, testPorts, testLogDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, chave := range []string{"vhosts/.dir", "tools/.dir"} {
		if _, ok := files[chave]; !ok {
			t.Errorf("falta %q; o Apache recusaria a config", chave)
		}
	}
}
