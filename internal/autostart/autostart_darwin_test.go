package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ligar grava o plist apontando para o executável atual; Enabled confere o
// caminho, para um plist de outra instalação não contar como ligado;
// desligar remove o arquivo e é idempotente.
func TestLaunchAgentLigaEDesliga(t *testing.T) {
	launchAgentsDir = t.TempDir()
	t.Cleanup(func() { launchAgentsDir = "" })
	if err := Apply(true); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(launchAgentsDir, plistName))
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	if !strings.Contains(string(raw), "<string>"+exe+"</string>") || !strings.Contains(string(raw), "<key>RunAtLoad</key>") {
		t.Fatalf("plist inesperado:\n%s", raw)
	}
	if on, err := Enabled(); err != nil || !on {
		t.Fatalf("Enabled = %v, %v", on, err)
	}
	if err := os.WriteFile(filepath.Join(launchAgentsDir, plistName), []byte("<string>/outro/HyPHP</string>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if on, _ := Enabled(); on {
		t.Fatal("plist de outro executável contou como ligado")
	}
	for range 2 {
		if err := Apply(false); err != nil {
			t.Fatal(err)
		}
	}
	if on, _ := Enabled(); on {
		t.Fatal("continua ligado depois de Apply(false)")
	}
}
