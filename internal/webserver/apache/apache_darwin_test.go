package apache

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hyphp/internal/runtime"
)

const (
	brewApacheDir = "/opt/homebrew/opt/httpd"
	// Com espaço, como o Application Support de verdade: tudo tem de ir entre aspas.
	brewLogDir = "/Users/joao/Library/Application Support/HyPHP/log"
)

func brewInstalled() runtime.Installed {
	return runtime.Installed{
		Kind:    runtime.Apache,
		Version: "2.4.62",
		Major:   "2.4.62",
		Dir:     brewApacheDir,
		Exe:     brewApacheDir + "/bin/httpd",
		Formula: "httpd",
		Prefix:  "/opt/homebrew",
	}
}

func renderBrew(t *testing.T, inst runtime.Installed) string {
	t.Helper()
	files, err := New(inst).Render(nil, testPools(), testPorts, brewLogDir, nil)
	if err != nil {
		t.Fatalf("Render erro = %v", err)
	}
	return string(files["httpd.conf"])
}

func TestRenderGoldenHomebrew(t *testing.T) {
	got := renderBrew(t, brewInstalled())
	path := filepath.Join("testdata", "darwin", "httpd.conf.golden")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler golden: %v", err)
	}
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if string(want) != got {
		os.WriteFile(path+".got", []byte(got), 0o644)
		t.Fatalf("httpd.conf difere do golden; gravei %s.got", path)
	}
}

// Sem MPM carregado o httpd do Homebrew recusa a config no -t, e o unixd
// precisa vir antes dos módulos que dependem dele.
func TestRenderHomebrewCarregaMPMAntesDosModulos(t *testing.T) {
	conf := renderBrew(t, brewInstalled())
	mpm := strings.Index(conf, "LoadModule mpm_event_module lib/httpd/modules/mod_mpm_event.so")
	unixd := strings.Index(conf, "LoadModule unixd_module lib/httpd/modules/mod_unixd.so")
	first := strings.Index(conf, "LoadModule authn_core_module")
	if mpm < 0 || unixd < 0 || mpm > first || unixd > first {
		t.Fatalf("mpm_event/unixd ausentes ou depois da lista comum:\n%s", conf)
	}
	if strings.Contains(conf, " modules/mod_") {
		t.Fatalf("módulo com o caminho do Windows:\n%s", conf)
	}
	if strings.Contains(conf, "[A-Za-z]:/") {
		t.Fatalf("ProxyFCGISetEnvIf da letra de drive não tem sentido no Mac:\n%s", conf)
	}
}

// O keg antigo sem Prefix gravado ainda acha o mime.types do Homebrew.
func TestRenderHomebrewSemPrefixUsaLayoutOpt(t *testing.T) {
	inst := brewInstalled()
	inst.Prefix = ""
	conf := renderBrew(t, inst)
	if !strings.Contains(conf, `TypesConfig "/opt/homebrew/etc/httpd/mime.types"`) {
		t.Fatalf("TypesConfig sem o prefixo deduzido de opt/<formula>:\n%s", conf)
	}
}

func TestCommandHomebrewFicaEmPrimeiroPlano(t *testing.T) {
	const etc = "/Users/joao/Library/Application Support/HyPHP/etc/apache.next"
	exe, args, dir := New(brewInstalled()).Command(etc)
	if exe != brewApacheDir+"/bin/httpd" || dir != brewApacheDir {
		t.Fatalf("exe = %q, dir = %q", exe, dir)
	}
	want := []string{
		"-f", etc + "/httpd.conf",
		"-d", brewApacheDir,
		"-C", `Define HYPHP_ETC "` + etc + `"`,
		"-D", "FOREGROUND",
	}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %q, quero %q", args, want)
	}
}

func TestValidateHomebrewSemForeground(t *testing.T) {
	_, args, _ := server{inst: brewInstalled()}.command("/tmp/etc/apache.next", true)
	if args[0] != "-t" {
		t.Fatalf("args = %q, quero -t primeiro", args)
	}
	if slicesContains(args, "FOREGROUND") {
		t.Fatalf("-t não pode levar -D FOREGROUND: %q", args)
	}
}
