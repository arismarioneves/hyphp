package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Instalador trocado entre Prepare e a execução elevada tem de ser recusado
// antes do UAC. PID 0 não abre processo, então waitExit não espera.
func TestApplyRecusaInstaladorComHashErrado(t *testing.T) {
	inst := filepath.Join(t.TempDir(), "setup.exe")
	if err := os.WriteFile(inst, []byte("trocado"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := ApplyRequest{Installer: inst, SHA256: strings.Repeat("0", 64), Size: int64(len("trocado"))}
	err := apply(req, func(string, ...any) {})
	if err == nil || !strings.Contains(err.Error(), "não confere") {
		t.Fatalf("esperava recusa por hash, veio %v", err)
	}
}
