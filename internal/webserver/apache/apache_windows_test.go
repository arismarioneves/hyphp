package apache

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func goldenName(key string) string {
	return strings.ReplaceAll(key, "/", "_") + ".golden"
}

func TestRenderGolden(t *testing.T) {
	files := renderFixture(t)

	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"default/dados/.keep", "default/index.html", "httpd.conf", "pools.conf", "tools/.dir", "vhosts/.dir", "vhosts/app72.conf", "vhosts/app81.conf"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("chaves = %v, quero %v", keys, want)
	}

	for _, key := range keys {
		// ".dir" e ".keep" só declaram o diretório; não têm conteúdo a comparar.
		if strings.HasSuffix(key, "/.dir") || strings.HasSuffix(key, "/.keep") {
			continue
		}
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

// O etcDir chega com o separador do SO (FromSlash: o "C:\..." de sempre no
// Windows) e os argumentos saem com "/" nos dois.
func TestCommandDefineEtc(t *testing.T) {
	exe, args, dir := New(testInstalled()).Command(filepath.FromSlash("C:/hyphp/etc/apache.next"))
	if exe != testApacheDir+"/bin/httpd.exe" {
		t.Fatalf("exe = %q", exe)
	}
	if dir != testApacheDir {
		t.Fatalf("dir = %q, quero o ServerRoot", dir)
	}
	want := []string{
		"-f", "C:/hyphp/etc/apache.next/httpd.conf",
		"-d", testApacheDir,
		"-C", `Define HYPHP_ETC "C:/hyphp/etc/apache.next"`,
	}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %q, quero %q", args, want)
	}
	if slicesContains(args, "-t") {
		t.Fatal("Command não pode passar -t: -t só valida e sai")
	}
}

// O Apache tokeniza o -C como uma linha de config: sem aspas, um perfil com
// espaço daria três argumentos ao Define e o httpd -t recusaria tudo.
func TestCommandDefineEtcComEspaco(t *testing.T) {
	_, args, _ := New(testInstalled()).Command(filepath.FromSlash("C:/Users/João Silva/HyPHP/etc/apache"))
	want := `Define HYPHP_ETC "C:/Users/João Silva/HyPHP/etc/apache"`
	if args[len(args)-1] != want {
		t.Fatalf("-C = %q, quero %q", args[len(args)-1], want)
	}
}
