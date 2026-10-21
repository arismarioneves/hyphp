package services

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"hyphp/internal/i18n"
	"hyphp/internal/state"
)

func appCom(st state.State) *AppService {
	return &AppService{d: AppDeps{State: func() state.State { return st }}}
}

// programaFalso põe no PATH (e só ele) um programa que grava os argumentos e a
// pasta de trabalho em saida.
func programaFalso(t *testing.T, nome string) (saida string) {
	t.Helper()
	dir := t.TempDir()
	saida = filepath.Join(dir, "saida.txt")
	script := "#!/bin/sh\n{ pwd; printf '%s\\n' \"$@\"; } > '" + saida + "'\n"
	if err := os.WriteFile(filepath.Join(dir, nome), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	return saida
}

func esperarSaida(t *testing.T, saida string) []string {
	t.Helper()
	for range 50 {
		if raw, err := os.ReadFile(saida); err == nil && len(raw) > 0 {
			return strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("o programa não rodou")
	return nil
}

// Sem editor configurado e sem o VS Code, o erro diz onde escolher outro, e não
// o "executable file not found in $PATH" do exec.
func TestEditorAusenteDizOndeEscolherOutro(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := appCom(state.State{}).openInEditor(t.TempDir()); err == nil || err.Error() != i18n.T("err.app.noEditor") {
		t.Fatalf("sem editor: err = %v", err)
	}
	if err := appCom(state.State{Editor: "zed"}).openInEditor(t.TempDir()); err == nil || err.Error() != i18n.T("err.app.editorNotFound", "zed") {
		t.Fatalf("editor configurado ausente: err = %v", err)
	}
	if err := appCom(state.State{Terminal: "alacritty"}).openTerminal(t.TempDir()); err == nil || err.Error() != i18n.T("err.app.terminalNotFound", "alacritty") {
		t.Fatalf("terminal configurado ausente: err = %v", err)
	}
	if err := appCom(state.State{}).openTerminal(t.TempDir()); err == nil || !strings.Contains(err.Error(), "ptyxis") {
		t.Fatalf("sem terminal: err = %v", err)
	}
}

// O Ptyxis (o terminal do GNOME no Ubuntu 25.10 em diante) só abre na pasta
// pedida numa janela nova; sem isso, traz a janela aberta, na pasta dela.
func TestPtyxisAbreUmaJanelaNovaNaPasta(t *testing.T) {
	saida := programaFalso(t, "ptyxis")
	pasta := t.TempDir()
	if err := appCom(state.State{}).openTerminal(pasta); err != nil {
		t.Fatal(err)
	}
	got := esperarSaida(t, saida)
	if got[0] != pasta || !slices.Equal(got[1:], []string{"--new-window", "--working-directory=" + pasta}) {
		t.Fatalf("pwd e argumentos = %q", got)
	}
}

// O x-terminal-emulator é um link para o terminal preferido: a pasta vai na
// opção do terminal de verdade.
func TestXTerminalEmulatorUsaAOpcaoDoTerminalDeVerdade(t *testing.T) {
	saida := programaFalso(t, "ptyxis")
	dir := filepath.Dir(saida)
	if err := os.Symlink(filepath.Join(dir, "ptyxis"), filepath.Join(dir, "x-terminal-emulator")); err != nil {
		t.Fatal(err)
	}
	pasta := t.TempDir()
	if err := appCom(state.State{}).openTerminal(pasta); err != nil {
		t.Fatal(err)
	}
	if got := esperarSaida(t, saida); !slices.Contains(got, "--new-window") {
		t.Fatalf("argumentos = %q", got)
	}
}
