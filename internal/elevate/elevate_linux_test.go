package elevate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pkexecFalso põe no PATH um pkexec que confere o --disable-internal-agent e
// roda o programa como o próprio usuário, ou sai com sair quando ≠ "".
func pkexecFalso(t *testing.T, sair string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n[ \"$1\" = --disable-internal-agent ] || exit 99\nshift\nexec \"$@\"\n"
	if sair != "" {
		script = "#!/bin/sh\necho 'pkexec: motivo' >&2\nexit " + sair + "\n"
	}
	escreverExe(t, filepath.Join(dir, "pkexec"), script)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func escreverExe(t *testing.T, path, script string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// helperFalso escreve um helper que grava os argumentos em args.txt, imprime
// saida e sai com codigo.
func helperFalso(t *testing.T, saida, codigo string) (exe, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args.txt")
	exe = filepath.Join(dir, "hyphp-helper")
	escreverExe(t, exe, "#!/bin/sh\nprintf '%s\\n' \"$@\" > '"+argsFile+"'\nprintf '%s\\n' '"+saida+"'\nexit "+codigo+"\n")
	return exe, argsFile
}

func TestRunElevatedPassaOsArgumentosIntactos(t *testing.T) {
	pkexecFalso(t, "")
	exe, argsFile := helperFalso(t, `{"ok":true}`, "0")
	args := []string{"profile-write", "--dir", "/home/joão silva/cli"}
	if err := RunElevated(exe, args); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Split(strings.TrimRight(string(raw), "\n"), "\n"); strings.Join(got, "|") != strings.Join(args, "|") {
		t.Fatalf("args = %q, quer %q", got, args)
	}
}

func TestRunElevatedDevolveOErroDoHelper(t *testing.T) {
	pkexecFalso(t, "")
	exe, _ := helperFalso(t, `{"ok":false,"error":"--dir pertence a root"}`, "2")
	var he *HelperError
	err := RunElevated(exe, nil)
	if !errors.As(err, &he) || he.ExitCode != 2 || he.Message != "--dir pertence a root" {
		t.Fatalf("err = %v", err)
	}
}

func TestRunElevatedJanelaCanceladaENegada(t *testing.T) {
	pkexecFalso(t, "126")
	if err := RunElevated("/bin/true", nil); !errors.Is(err, ErrElevationDenied) {
		t.Fatalf("err = %v, quer ErrElevationDenied", err)
	}
}

// 127 é o pkexec sem conseguir pedir a senha (sem agente do polkit, sem
// autorização): o motivo dele vai junto, e não vira cancelamento.
func TestRunElevatedFalhaDoPkexecTrazOMotivo(t *testing.T) {
	pkexecFalso(t, "127")
	err := RunElevated("/bin/true", nil)
	if err == nil || errors.Is(err, ErrElevationDenied) || !strings.Contains(err.Error(), "pkexec: motivo") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunElevatedSemPkexecNoSistema(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := RunElevated("/bin/true", nil)
	var he *HelperError
	if err == nil || errors.As(err, &he) || errors.Is(err, ErrElevationDenied) {
		t.Fatalf("err = %v", err)
	}
}

// A janela de senha esquecida aberta: o pkexec, ainda do usuário, é
// encerrado no prazo, sem esperar o resto.
func TestRunElevatedJanelaEsquecidaPassaDoPrazo(t *testing.T) {
	dir := t.TempDir()
	escreverExe(t, filepath.Join(dir, "pkexec"), "#!/bin/sh\nexec sleep 30\n")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	inicio := time.Now()
	if err := runElevated("/bin/true", nil, 200*time.Millisecond); !errors.Is(err, ErrHelperTimeout) {
		t.Fatalf("err = %v, quer ErrHelperTimeout", err)
	}
	if d := time.Since(inicio); d > 10*time.Second {
		t.Fatalf("levou %s para desistir", d)
	}
}

func TestHelperPathFicaAoLadoDoApp(t *testing.T) {
	dir := t.TempDir()
	if _, err := helperPathIn(dir); err == nil {
		t.Fatal("sem o helper, HelperPath devia falhar")
	}
	escreverExe(t, filepath.Join(dir, "hyphp-helper"), "#!/bin/sh\n")
	if got, err := helperPathIn(dir); err != nil || got != filepath.Join(dir, "hyphp-helper") {
		t.Fatalf("helperPathIn = %q, %v", got, err)
	}
}
