package stack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

// TestSplitCommandRoundTrip prova que splitCommand é o inverso de
// syscall.EscapeArg — o mesmo escape que o os/exec usa para montar a linha de
// comando no Windows. É o teste que pega regressão de contagem de barras.
func TestSplitCommandRoundTrip(t *testing.T) {
	cases := [][]string{
		{"php", "artisan", "queue:work", "--tries=3"},
		{"node", `C:\Program Files\nodejs\x.js`, "--flag"},
		{"cmd", `C:\temp\`, "fim"},
		{"echo", `aspas "no meio"`},
		{"echo", ""},
		{"a b", "c\td"},
		{`barra\dupla\\`, "x"},
	}
	for _, args := range cases {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = syscall.EscapeArg(a)
		}
		line := strings.Join(parts, " ")
		if got := splitCommand(line); !reflect.DeepEqual(got, args) {
			t.Fatalf("splitCommand(%q) = %q, want %q", line, got, args)
		}
	}
}

func TestDefaultLookPathIn(t *testing.T) {
	dir := t.TempDir()
	bat := filepath.Join(dir, "tool.bat")
	if err := os.WriteFile(bat, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := defaultLookPathIn(dir, "tool")
	if err != nil || got != bat {
		t.Fatalf("defaultLookPathIn = %q, %v; want %q", got, err, bat)
	}
	if _, err := defaultLookPathIn(dir, "inexistente"); err == nil {
		t.Fatal("esperava erro para arquivo inexistente")
	}
	if got, err := defaultLookPathIn("", bat); err != nil || got != bat {
		t.Fatalf("caminho absoluto = %q, %v", got, err)
	}
}
