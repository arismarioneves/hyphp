package elevate

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Nenhum texto de fora (prompt, caminho, argumento) entra no código
// AppleScript: tudo vai em argv, depois do "--". Um argumento com aspas ou
// ponto e vírgula viraria comando rodando como root.
func TestArgumentosDoOsascriptVaoEmArgv(t *testing.T) {
	args := osascriptArgs("Senha \"x\"", "/Apps/HyPHP.app/Contents/Helpers/hyphp-helper", []string{"paths-write", "--dir", "/a b/c'd; rm -rf /"})
	i := len(args) - 6 // "--", prompt, exe e os 3 argumentos
	if args[i] != "--" {
		t.Fatalf("args sem \"--\" antes de argv: %q", args)
	}
	for _, a := range args[:i] {
		if strings.Contains(a, "rm -rf") || strings.Contains(a, "Senha") {
			t.Fatalf("texto de fora dentro do script: %q", a)
		}
	}
}

// O script de verdade, sem o "with administrator privileges" que pediria
// senha: argumentos com espaço e aspas chegam intactos ao processo, e o
// código de saída e o JSON voltam ao Go.
func TestDoShellScriptDevolveJSONECodigo(t *testing.T) {
	args := osascriptArgs("x", "/bin/sh", []string{"-c", `printf '{"ok":false,"error":"a b'"'"'c"}'; exit 3`})
	for i, a := range args {
		args[i] = strings.Replace(a, " with administrator privileges", "", 1)
	}
	out, err := exec.Command("/usr/bin/osascript", args...).Output()
	if err != nil {
		t.Fatalf("osascript: %v", err)
	}
	var he *HelperError
	if err := helperResult(string(out)); !errors.As(err, &he) || he.ExitCode != 3 || he.Message != "a b'c" {
		t.Fatalf("helperResult = %v, quer código 3 e mensagem \"a b'c\"", err)
	}
}

func TestResultadoDoHelper(t *testing.T) {
	if err := helperResult("{\"ok\":true}\n\nexit=0"); err != nil {
		t.Fatalf("sucesso virou erro: %v", err)
	}
	var he *HelperError
	if err := helperResult("{\"ok\":false,\"error\":\"--port fora\"}\n\nexit=2"); !errors.As(err, &he) || he.ExitCode != 2 || he.Message != "--port fora" {
		t.Fatalf("erro do helper = %v", err)
	}
	if err := helperResult("sem código"); !errors.Is(err, ErrHelperFailed) {
		t.Fatalf("saída sem código = %v, quer ErrHelperFailed", err)
	}
}

// Fechar a janela de senha é recusa, como cancelar o UAC no Windows.
func TestCancelarAJanelaDeSenha(t *testing.T) {
	if err := osascriptError("0:120: execution error: User canceled. (-128)\n"); !errors.Is(err, ErrElevationDenied) {
		t.Fatalf("cancelamento = %v", err)
	}
	if err := osascriptError("0:10: syntax error"); !errors.Is(err, ErrHelperFailed) {
		t.Fatalf("outra falha = %v, quer ErrHelperFailed", err)
	}
}

// O helper fica em Contents/Helpers, ao lado da CLI; sem ele, erro (build de
// dev sem build:helper).
func TestHelperPathNoBundle(t *testing.T) {
	app := filepath.Join(t.TempDir(), "HyPHP.app", "Contents")
	exe := filepath.Join(app, "MacOS", "hyphp")
	if _, err := helperPathFor(exe); err == nil {
		t.Fatal("sem helper no bundle não deu erro")
	}
	helper := filepath.Join(app, "Helpers", "hyphp-helper")
	if err := os.MkdirAll(filepath.Dir(helper), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helper, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := helperPathFor(exe); err != nil || got != helper {
		t.Fatalf("helperPathFor = (%q, %v), quer %q", got, err, helper)
	}
}
