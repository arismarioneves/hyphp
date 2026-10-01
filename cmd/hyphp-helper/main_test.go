//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"

	"hyphp/internal/paths"
)

// Elevado, o helper não pode servir de lançador genérico: só o mkcert.exe da
// raiz passa da validação, e a recusa vem antes de qualquer execução.
func TestMkcertInstallRecusaExeForaDoBin(t *testing.T) {
	root := t.TempDir()
	t.Setenv(paths.EnvRoot, root)
	outro := filepath.Join(t.TempDir(), "mkcert.exe")
	renomeado := filepath.Join(root, "bin", "mkcert", "calc.exe")
	escapando := filepath.Join(root, "bin", "mkcert", "..", "mkcert.exe")
	for _, exe := range []string{outro, renomeado, escapando} {
		if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(exe, []byte("MZ"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, err := mkcertInstall([]string{"--exe", exe})
		if code != exitUsage || err == nil {
			t.Errorf("%s: code=%d err=%v; esperava recusa com exitUsage", exe, code, err)
		}
	}
}

func TestUnderAceitaSoODiretorioDaRaiz(t *testing.T) {
	root := t.TempDir()
	run := filepath.Join(root, "var", "run")
	cases := []struct {
		p    string
		want bool
	}{
		{filepath.Join(run, "helper-0123456789abcdef.json"), true},
		{filepath.Join(run, "sub", "x.json"), true},
		{run, false},
		{filepath.Join(run, "..", "x.json"), false},
		{filepath.Join(root, "var", "runx", "x.json"), false},
		{`C:\Windows\System32\drivers\etc\hosts`, false},
	}
	for _, c := range cases {
		if got := under(c.p, run); got != c.want {
			t.Errorf("under(%q) = %v, quero %v", c.p, got, c.want)
		}
	}
}
