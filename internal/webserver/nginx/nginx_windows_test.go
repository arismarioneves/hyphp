package nginx

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestRenderGolden(t *testing.T) {
	files := renderFixture(t)

	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{
		"html/dados/.keep", "html/index.html", "logs/.keep", "nginx.conf",
		"sites/.dir", "sites/app72.conf", "sites/app81.conf", "temp/.keep", "upstreams.conf",
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

// O etcDir chega com o separador do SO (FromSlash: o "C:\..." de sempre no
// Windows) e o -p sai com "/" nos dois.
func TestCommandEValidateUsamPrefixo(t *testing.T) {
	exe, args, dir := New(testInstalled()).Command(filepath.FromSlash("C:/hyphp/etc/nginx.next"))
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
