package services

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

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

// Editor/terminal copiados de um atalho vêm com %VAR%; sem expandir, o start
// procura um arquivo chamado literalmente "%LOCALAPPDATA%\...". Variável
// inexistente fica intacta, como no Windows.
func TestExpandEnvNoCaminhoDoEditor(t *testing.T) {
	t.Setenv("HYPHP_TESTE_APPS", `C:\Users\fulano\AppData\Local`)
	cases := map[string]string{
		`%HYPHP_TESTE_APPS%\Programs\Microsoft VS Code\Code.exe`: `C:\Users\fulano\AppData\Local\Programs\Microsoft VS Code\Code.exe`,
		`%HYPHP_TESTE_NADA%\x.exe`:                               `%HYPHP_TESTE_NADA%\x.exe`,
		`wt.exe`:                                                 `wt.exe`,
	}
	for in, want := range cases {
		if got := expandEnv(in); got != want {
			t.Errorf("expandEnv(%q) = %q, want %q", in, got, want)
		}
	}
}
