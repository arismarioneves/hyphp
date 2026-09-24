package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoot(t *testing.T) {
	custom := t.TempDir()

	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "HYPHP_ROOT definido vence", env: custom, want: custom},
		{name: "HYPHP_ROOT relativo vira absoluto", env: ".", want: mustAbs(t, ".")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvRoot, tt.env)
			if got := Root(); got != tt.want {
				t.Fatalf("Root() = %q, want %q", got, tt.want)
			}
		})
	}
}

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

// Uma vez que os dados moram em LOCALAPPDATA, nenhuma outra regra pode mudar a
// raiz — senão a mesma máquina teria dois ambientes.
func TestRootPrefereRaizJaExistente(t *testing.T) {
	orig := writable
	t.Cleanup(func() { writable = orig })
	writable = func(string) bool { return true }

	local := t.TempDir()
	raiz := filepath.Join(local, "HyPHP")
	if err := os.MkdirAll(filepath.Join(raiz, "var"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(raiz, "var", "state.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvRoot, "")
	t.Setenv("LOCALAPPDATA", local)

	if got := Root(); got != raiz {
		t.Errorf("Root() = %q, quero %q", got, raiz)
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

func TestSubdirsDerivamDeRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvRoot, root)

	tests := []struct {
		name string
		fn   func() string
		want string
	}{
		{"Bin", Bin, filepath.Join(root, "bin")},
		{"Etc", Etc, filepath.Join(root, "etc")},
		{"Var", Var, filepath.Join(root, "var")},
		{"Log", Log, filepath.Join(root, "log")},
		{"Run", Run, filepath.Join(root, "var", "run")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(); got != tt.want {
				t.Fatalf("%s() = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestEnsureLayout(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvRoot, root)

	if err := EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	for _, rel := range []string{"bin", "etc", "var", filepath.Join("var", "run"), "log"} {
		info, err := os.Stat(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("%s nao criado: %v", rel, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s nao e diretorio", rel)
		}
	}
	// idempotente
	if err := EnsureLayout(); err != nil {
		t.Fatalf("segunda chamada: %v", err)
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
