//go:build windows

package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootUsaLocalAppDataQuandoExeNaoEGravavel(t *testing.T) {
	// A sonda de gravabilidade é o que decide; injetá-la mantém o teste
	// determinístico sem precisar de um diretório realmente protegido.
	orig := writable
	t.Cleanup(func() { writable = orig })
	writable = func(string) bool { return false }

	t.Setenv(EnvRoot, "")
	t.Setenv("LOCALAPPDATA", `C:\Users\teste\AppData\Local`)

	if got, want := Root(), `C:\Users\teste\AppData\Local\HyPHP`; got != want {
		t.Errorf("Root() = %q, quero %q", got, want)
	}
}

func TestRootPrefereExeGravavel(t *testing.T) {
	orig := writable
	t.Cleanup(func() { writable = orig })
	writable = func(string) bool { return true }

	t.Setenv(EnvRoot, "")
	// LOCALAPPDATA aponta para um diretório vazio: sem raiz anterior, a regra
	// do diretório do executável é a que vale.
	t.Setenv("LOCALAPPDATA", t.TempDir())
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := Root(), filepath.Dir(exe); got != want {
		t.Errorf("Root() = %q, quero %q", got, want)
	}
}

// A raiz não pode depender do token do processo: Program Files é gravável para
// um processo elevado, e abrir o app como administrador passava a usar o
// diretório de instalação — o usuário via tudo desaparecer.
func TestRootIgnoraProgramFilesMesmoGravavel(t *testing.T) {
	orig := writable
	t.Cleanup(func() { writable = orig })
	writable = func(string) bool { return true } // como se fosse elevado

	local := t.TempDir()
	t.Setenv(EnvRoot, "")
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("ProgramFiles", filepath.Dir(mustExeDir(t)))

	if got, want := Root(), filepath.Join(local, "HyPHP"); got != want {
		t.Errorf("Root() = %q, quero %q", got, want)
	}
}

// Instalado fora das áreas do sistema, a pasta da instalação é a raiz — mesmo
// que exista uma raiz antiga em LOCALAPPDATA. Preferir a antiga faria o
// usuário instalar runtimes numa raiz e o app ler a outra.
func TestRootPrefereInstalacaoAJaExistenteEmLocalAppData(t *testing.T) {
	orig := writable
	t.Cleanup(func() { writable = orig })
	writable = func(string) bool { return true }

	local := t.TempDir()
	antiga := filepath.Join(local, "HyPHP")
	if err := os.MkdirAll(filepath.Join(antiga, "var"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(antiga, "var", "state.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvRoot, "")
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("ProgramFiles", filepath.Join(t.TempDir(), "sem-relacao"))

	if got, want := Root(), mustExeDir(t); got != want {
		t.Errorf("Root() = %q, quero %q", got, want)
	}
}

func mustExeDir(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(exe)
}

func TestRootEnvVenceTudo(t *testing.T) {
	orig := writable
	t.Cleanup(func() { writable = orig })
	writable = func(string) bool { return false }

	t.Setenv(EnvRoot, `C:\hyphp-teste`)
	if got, want := Root(), `C:\hyphp-teste`; got != want {
		t.Errorf("Root() = %q, quero %q", got, want)
	}
}

// Depois de Freeze, uma falha passageira da sonda de escrita não pode mudar a
// raiz no meio da sessão.
func TestFreezeFixaARaiz(t *testing.T) {
	orig := writable
	t.Cleanup(func() { writable = orig; frozen = "" })
	writable = func(string) bool { return true }
	t.Setenv(EnvRoot, "")
	t.Setenv("LOCALAPPDATA", t.TempDir())

	Freeze()
	want := mustExeDir(t)
	writable = func(string) bool { return false }
	t.Setenv(EnvRoot, t.TempDir())
	if got := Root(); got != want {
		t.Errorf("Root() depois de Freeze = %q, quero %q", got, want)
	}
}
