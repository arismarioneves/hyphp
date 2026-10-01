package services

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"hyphp/internal/paths"
	"hyphp/internal/state"
)

func newTestService(t *testing.T) *AppService {
	t.Helper()
	return NewAppService(AppDeps{
		Quit:   func() {},
		State:  state.Default,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
}

// OpenExternal só pode abrir http/https: aceitar file:// ou um scheme arbitrário
// transformaria um domínio vindo de hyphp.yaml em execução de caminho local.
func TestOpenExternalRecusaEsquemaNaoWeb(t *testing.T) {
	a := newTestService(t)
	tests := []struct {
		name string
		url  string
	}{
		{"file local", `file:///C:/Windows/System32/calc.exe`},
		{"sem esquema", "example.test"},
		{"esquema arbitrario", "javascript:alert(1)"},
		{"http sem host", "http://"},
		{"vazio", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := a.OpenExternal(tt.url)
			if err == nil {
				t.Fatalf("OpenExternal(%q) devia falhar", tt.url)
			}
			if !strings.Contains(err.Error(), "url invalida") {
				t.Fatalf("erro inesperado: %v", err)
			}
		})
	}
}

// OpenFolder/OpenTerminal recebem caminho vindo da UI; arquivo ou caminho
// inexistente não pode virar chamada ao Explorer com argumento arbitrário.
func TestAberturaDeDiretorioValidaCaminho(t *testing.T) {
	a := newTestService(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "arquivo.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "nao-existe")

	tests := []struct {
		name    string
		path    string
		wantSub string
	}{
		{"inexistente", missing, "diretorio inexistente"},
		{"arquivo em vez de diretorio", file, "nao e diretorio"},
	}
	for _, tt := range tests {
		t.Run("OpenFolder/"+tt.name, func(t *testing.T) {
			err := a.OpenFolder(tt.path)
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("OpenFolder(%q) = %v, queria conter %q", tt.path, err, tt.wantSub)
			}
		})
		t.Run("OpenTerminal/"+tt.name, func(t *testing.T) {
			err := a.OpenTerminal(tt.path)
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("OpenTerminal(%q) = %v, queria conter %q", tt.path, err, tt.wantSub)
			}
		})
	}

	t.Run("OpenInEditor/inexistente", func(t *testing.T) {
		err := a.OpenInEditor(missing)
		if err == nil || !strings.Contains(err.Error(), "caminho inexistente") {
			t.Fatalf("OpenInEditor(%q) = %v", missing, err)
		}
	})
}

func TestRuntimeRootSegueHYPHPROOT(t *testing.T) {
	root := t.TempDir()
	t.Setenv(paths.EnvRoot, root)
	if got := newTestService(t).RuntimeRoot(); got != root {
		t.Fatalf("RuntimeRoot() = %q, want %q", got, root)
	}
}

// A montagem do novo PATH é a parte que pode corromper o ambiente do usuário,
// e é pura — o teste cobre exatamente ela. O registro em si fica de fora: o
// teste não pode mexer no HKCU\Environment da máquina de quem roda a suíte.
func TestPathComDirNaoDuplica(t *testing.T) {
	const dir = `C:\hyphp\bin\php\php-8.1.10`
	atual := `C:\Windows;` + dir + `;C:\Git\cmd`

	if got, mudou := pathComDir(atual, dir); mudou {
		t.Errorf("mudou = true para PATH que já contém o diretório; got %q", got)
	}
}

func TestPathComDirAcrescentaNoInicio(t *testing.T) {
	const dir = `C:\hyphp\bin\php\php-8.1.10`
	got, mudou := pathComDir(`C:\Windows;C:\Git\cmd`, dir)
	if !mudou {
		t.Fatal("mudou = false")
	}
	// No início: se houver outro php.exe no PATH (de outra ferramenta), o
	// nosso precisa vencer, senão o toggle não muda nada na prática.
	if want := dir + `;C:\Windows;C:\Git\cmd`; got != want {
		t.Errorf("got %q, quero %q", got, want)
	}
}

// PATH vazio é possível num perfil novo e não pode virar ";dir".
func TestPathComDirVazio(t *testing.T) {
	got, mudou := pathComDir("", `C:\php`)
	if !mudou || got != `C:\php` {
		t.Errorf("got %q, %v", got, mudou)
	}
}

// Sem editor configurado o caminho passa pelo cmd.exe: fora de aspas, `&`,
// `^` e parênteses de um nome de pasta viram sintaxe do cmd. Depois do cmd,
// o code.cmd tem de receber o caminho intacto como um argumento só.
func TestEditorCmdLineProtegeCaminhoDoCmd(t *testing.T) {
	for _, p := range []string{`C:\www\a&b`, `C:\www\x^y`, `C:\www\app (1)`, `C:\www\com espaço`, `C:\`} {
		line := editorCmdLine(p)
		prefix := `cmd.exe /c code "`
		if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, `"`) {
			t.Errorf("editorCmdLine(%q) = %q: caminho fora de aspas", p, line)
			continue
		}
		args, err := windows.DecomposeCommandLine(line)
		if err != nil {
			t.Fatal(err)
		}
		if len(args) != 4 || args[3] != p {
			t.Errorf("editorCmdLine(%q) se decompõe em %q", p, args)
		}
	}
}
