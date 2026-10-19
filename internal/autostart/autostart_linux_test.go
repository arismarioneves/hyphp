package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ligar grava o .desktop que abre o executável atual com a Flag; Enabled só
// conta o arquivo que aponta para este executável; desligar remove e é
// idempotente.
func TestAutostartNoLinuxLigaEDesliga(t *testing.T) {
	autostartDir = t.TempDir()
	t.Cleanup(func() { autostartDir = "" })
	if err := Apply(true); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(autostartDir, desktopName))
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	if !strings.Contains(string(raw), "\nExec=\""+exe+"\" "+Flag+"\n") {
		t.Fatalf(".desktop inesperado:\n%s", raw)
	}
	if on, err := Enabled(); err != nil || !on {
		t.Fatalf("Enabled = %v, %v", on, err)
	}
	if err := os.WriteFile(filepath.Join(autostartDir, desktopName), []byte("[Desktop Entry]\nExec=\"/outro/hyphp\" --bandeja\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if on, _ := Enabled(); on {
		t.Fatal(".desktop de outro executável contou como ligado")
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

// Espaço fica protegido pelas aspas; % viraria código de campo e ", $ e \
// fechariam ou escapariam as aspas.
func TestExecDoDesktopEscapaOCaminho(t *testing.T) {
	casos := map[string]string{
		"/opt/meu app/100%/hyphp": `Exec="/opt/meu app/100%%/hyphp" --bandeja`,
		`/opt/a"b$c/hyphp`:        `Exec="/opt/a\\"b\\$c/hyphp" --bandeja`,
		`/opt/a\b/hyphp`:          `Exec="/opt/a\\\\b/hyphp" --bandeja`,
	}
	for exe, quer := range casos {
		if got := execLine(exe); got != quer {
			t.Errorf("execLine(%q) = %s; quero %s", exe, got, quer)
		}
	}
}
