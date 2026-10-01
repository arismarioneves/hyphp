package pkgmgr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Entrada com letra de unidade só escapa do destino no Windows; no macOS
// "C:/evil.txt" é uma pasta "C:" comum dentro do destino.
func TestExtractZipRecusaLetraDeUnidade(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "dest")
	r := buildZip(t, map[string]string{"ok/fine.txt": "x", `C:\evil.txt`: "pwned"})
	err := extractZip(r, dest, "")
	if err == nil || !strings.Contains(err.Error(), "zip-slip") {
		t.Fatalf("esperava erro zip-slip, veio %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(parent, "evil.txt")); statErr == nil {
		t.Fatal("entrada escreveu fora do destino")
	}
}
