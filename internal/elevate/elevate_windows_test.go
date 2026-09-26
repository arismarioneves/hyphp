package elevate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestShellExecuteInfoLayout(t *testing.T) {
	var info SHELLEXECUTEINFO
	if got, want := unsafe.Sizeof(info), uintptr(112); got != want {
		t.Fatalf("sizeof(SHELLEXECUTEINFO) = %d, want %d (layout amd64 de SHELLEXECUTEINFOW)", got, want)
	}
	if got, want := unsafe.Offsetof(info.nShow), uintptr(48); got != want {
		t.Fatalf("offset de nShow = %d, want %d", got, want)
	}
	if got, want := unsafe.Offsetof(info.hProcess), uintptr(104); got != want {
		t.Fatalf("offset de hProcess = %d, want %d", got, want)
	}
}

func TestQuoteArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"sem argumentos", nil, ""},
		{"argumento simples", []string{"nrpt-remove"}, "nrpt-remove"},
		{"dois simples", []string{"--namespace", ".test"}, "--namespace .test"},
		{"com espaço", []string{"--from", `C:\Users\Ana Paula\AppData\Local\Temp\hosts.render`}, `--from "C:\Users\Ana Paula\AppData\Local\Temp\hosts.render"`},
		{"com aspas", []string{`di"z`}, `di\"z`},
		{"espaço e aspas", []string{`um "dois" tres`}, `"um \"dois\" tres"`},
		{"argumento vazio", []string{"hosts-write", ""}, `hosts-write ""`},
		{"barra final sem espaço", []string{`C:\dir\`, "x"}, `C:\dir\ x`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quoteArgs(tt.args); got != tt.want {
				t.Fatalf("\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}

func TestHelperPathIn(t *testing.T) {
	dir := t.TempDir()

	if _, err := helperPathIn(dir); err == nil {
		t.Fatal("esperava erro com o diretório sem hyphp-helper.exe")
	} else if !strings.Contains(err.Error(), "hyphp-helper.exe") {
		t.Fatalf("erro deveria nomear o arquivo procurado: %v", err)
	}

	want := filepath.Join(dir, "hyphp-helper.exe")
	if err := os.WriteFile(want, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := helperPathIn(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestHelperPathSemHelperAoLado(t *testing.T) {
	// O binário de teste roda num diretório temporário do `go build`, onde não existe helper.
	if _, err := HelperPath(); err == nil {
		t.Skip("existe um hyphp-helper.exe ao lado do binário de teste; nada a verificar")
	} else if !strings.Contains(err.Error(), "hyphp-helper.exe") {
		t.Fatalf("erro deveria nomear o arquivo procurado: %v", err)
	}
}

func TestShellExecuteWaitExitCode(t *testing.T) {
	comspec := os.Getenv("ComSpec")
	if comspec == "" {
		comspec = `C:\Windows\System32\cmd.exe`
	}
	// Verbo "open": mesma mecânica de RunElevated (ShellExecuteEx → espera → exit code), sem UAC.
	code, err := shellExecuteWait("open", comspec, quoteArgs([]string{"/c", "exit 7"}), helperTimeoutMS)
	if err != nil {
		t.Fatal(err)
	}
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
}

func TestShellExecuteWaitArquivoInexistente(t *testing.T) {
	_, err := shellExecuteWait("open", filepath.Join(t.TempDir(), "nao-existe.exe"), "", helperTimeoutMS)
	if err == nil {
		t.Fatal("esperava erro para executável inexistente")
	}
	if errors.Is(err, ErrElevationDenied) {
		t.Fatalf("arquivo ausente não é elevação negada: %v", err)
	}
}

func TestHelperErrorIsErrHelperFailed(t *testing.T) {
	var err error = &HelperError{ExitCode: 3, Message: "hosts-write: gravar: acesso negado"}
	if !errors.Is(err, ErrHelperFailed) {
		t.Fatal("HelperError precisa satisfazer errors.Is(err, ErrHelperFailed)")
	}
	var he *HelperError
	if !errors.As(err, &he) || he.ExitCode != 3 {
		t.Fatalf("errors.As não recuperou o exit code: %v", err)
	}
	if !strings.Contains(err.Error(), "3") || !strings.Contains(err.Error(), "acesso negado") {
		t.Fatalf("mensagem deve citar código e motivo: %q", err.Error())
	}
}
