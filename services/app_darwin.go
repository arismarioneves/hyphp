package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"hyphp/internal/i18n"
)

// shellOpen entrega pasta e URL ao `open`, que escolhe o app padrão do
// sistema, como o ShellExecute faz no Windows.
func shellOpen(target string) error { return start(exec.Command("open", target)) }

// expandEnv aceita ~ e $VAR, a forma de caminho que se copia do Terminal.
func expandEnv(s string) string {
	if s == "~" || strings.HasPrefix(s, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			s = filepath.Join(home, strings.TrimPrefix(s, "~"))
		}
	}
	return os.ExpandEnv(s)
}

// openInEditor: um .app abre pelo `open -a`, que encontra o app pelo nome ou
// pelo caminho; outro valor é um executável. Vazio cai no VS Code, como no
// Windows.
func (a *AppService) openInEditor(path string) error {
	editor := expandEnv(strings.TrimSpace(a.d.State().Editor))
	switch {
	case editor == "":
		return start(exec.Command("open", "-a", "Visual Studio Code", path))
	case strings.HasSuffix(editor, ".app"):
		return start(exec.Command("open", "-a", editor, path))
	default:
		return start(exec.Command(editor, path))
	}
}

// openTerminal abre o app de terminal já na pasta; vazio usa o Terminal.
func (a *AppService) openTerminal(path string) error {
	term := expandEnv(strings.TrimSpace(a.d.State().Terminal))
	if term == "" {
		term = "Terminal"
	}
	return start(exec.Command("open", "-a", term, path))
}

// readUserPath: o app aberto pelo Finder não herda o PATH do shell, e ler o
// do shell é trabalho da M2. Até lá a aba CLI mostra "fora do PATH".
func readUserPath() (string, error) { return "", nil }

// addToUserPath recusa até a M2 (precisa gravar em /etc/paths.d com senha de
// admin); o botão mostra este erro em vez de fingir que gravou.
func addToUserPath(string) error { return i18n.Errorf("err.mac.unavailable") }

// pathComDir é a mesma conta do Windows com a regra do Unix: separador ":"
// e comparação exata, porque o APFS pode diferenciar maiúsculas. Info a usa
// para dizer se a CLI já está no PATH.
func pathComDir(atual, dir string) (string, bool) {
	for _, p := range strings.Split(atual, ":") {
		if strings.TrimRight(strings.TrimSpace(p), "/") == strings.TrimRight(dir, "/") {
			return atual, false
		}
	}
	if strings.TrimSpace(atual) == "" {
		return dir, true
	}
	return dir + ":" + atual, true
}
