package services

import (
	"path/filepath"
	"testing"
)

// Caminho de editor copiado do Terminal costuma vir com ~ ou $HOME; sem
// expandir, o open recebe um caminho que não existe.
func TestExpandEnvNoMac(t *testing.T) {
	t.Setenv("HOME", "/Users/teste")
	t.Setenv("EDITOR_DIR", "/opt/editor")
	casos := map[string]string{
		"~/bin/subl":         filepath.Join("/Users/teste", "bin/subl"),
		"$EDITOR_DIR/run":    "/opt/editor/run",
		"/usr/local/bin/zed": "/usr/local/bin/zed",
	}
	for in, want := range casos {
		if got := expandEnv(in); got != want {
			t.Errorf("expandEnv(%q) = %q, want %q", in, got, want)
		}
	}
}
