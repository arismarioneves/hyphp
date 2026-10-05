package nginx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hyphp/internal/runtime"
	"hyphp/internal/webserver"
)

const (
	brewNginxDir = "/opt/homebrew/opt/nginx"
	// Com espaço, como o Application Support de verdade.
	brewLogDir = "/Users/joao/Library/Application Support/HyPHP/log"
)

func brewInstalled() runtime.Installed {
	return runtime.Installed{
		Kind:    runtime.Nginx,
		Version: "1.27.2",
		Major:   "1.27.2",
		Dir:     brewNginxDir,
		Exe:     brewNginxDir + "/bin/nginx",
		Formula: "nginx",
		Prefix:  "/opt/homebrew",
	}
}

func brewSites() []webserver.Site {
	return []webserver.Site{{
		ID:       "app81",
		Domain:   "app81.test",
		Docroot:  "/Users/joao/Sites/app81/public",
		PoolName: "php81",
	}}
}

func TestRenderGoldenHomebrew(t *testing.T) {
	files, err := New(brewInstalled()).Render(brewSites(), testPools(), testPorts, brewLogDir, nil)
	if err != nil {
		t.Fatalf("Render erro = %v", err)
	}
	for _, key := range []string{"nginx.conf", "sites/app81.conf"} {
		t.Run(key, func(t *testing.T) {
			path := filepath.Join("testdata", "darwin", strings.ReplaceAll(key, "/", "_")+".golden")
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ler golden: %v", err)
			}
			want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
			if !bytes.Equal(want, files[key]) {
				os.WriteFile(path+".got", files[key], 0o644)
				t.Fatalf("%s difere do golden; gravei %s.got", key, path)
			}
		})
	}
}

// O keg antigo sem Prefix gravado ainda acha o fastcgi_params do Homebrew.
func TestRenderHomebrewSemPrefixUsaLayoutOpt(t *testing.T) {
	inst := brewInstalled()
	inst.Prefix = ""
	files, err := New(inst).Render(brewSites(), testPools(), testPorts, brewLogDir, nil)
	if err != nil {
		t.Fatalf("Render erro = %v", err)
	}
	want := `include "/opt/homebrew/etc/nginx/fastcgi_params";`
	if !strings.Contains(string(files["sites/app81.conf"]), want) {
		t.Fatalf("site sem %s:\n%s", want, files["sites/app81.conf"])
	}
}

// -e vale para o -t também: o log de erro compilado é aberto antes da config.
func TestCommandEValidateHomebrewRedirecionamLogDeErro(t *testing.T) {
	const etc = "/Users/joao/Library/Application Support/HyPHP/etc/nginx.next"
	exe, args, dir := New(brewInstalled()).Command(etc)
	if exe != brewNginxDir+"/bin/nginx" || dir != brewNginxDir {
		t.Fatalf("exe = %q, dir = %q", exe, dir)
	}
	want := []string{"-p", etc + "/", "-c", "nginx.conf", "-e", "logs/error.log", "-g", "daemon off;"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("Command args = %q, quero %q", args, want)
	}
	_, args, _ = server{inst: brewInstalled()}.command(etc, true)
	want = []string{"-t", "-p", etc + "/", "-c", "nginx.conf", "-e", "logs/error.log"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("Validate args = %q, quero %q", args, want)
	}
}
