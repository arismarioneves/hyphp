package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"hyphp/internal/i18n"
	"hyphp/internal/paths"
)

// shellOpen entrega pasta e URL ao xdg-open, que escolhe o app padrão da
// sessão, como o ShellExecute faz no Windows.
func shellOpen(target string) error { return start(exec.Command("xdg-open", target)) }

// openInEditor executa o editor configurado com a pasta; vazio cai no VS
// Code (`code` no PATH), como no Windows.
func (a *AppService) openInEditor(path string) error {
	editor := expandEnv(strings.TrimSpace(a.d.State().Editor))
	if editor == "" {
		editor = "code"
	}
	return start(exec.Command(editor, path))
}

// terminais são os emuladores tentados quando o usuário não configurou um,
// com a opção que abre já na pasta. x-terminal-emulator é o padrão do
// Debian e derivados; os outros cobrem GNOME, KDE e Xfce.
var terminais = []struct{ exe, dirFlag string }{
	{"x-terminal-emulator", "--working-directory"},
	{"gnome-terminal", "--working-directory"},
	{"konsole", "--workdir"},
	{"xfce4-terminal", "--working-directory"},
}

// openTerminal abre o terminal já na pasta. O configurado roda com a pasta
// como diretório de trabalho; vazio usa o primeiro da lista que existir.
func (a *AppService) openTerminal(path string) error {
	if term := expandEnv(strings.TrimSpace(a.d.State().Terminal)); term != "" {
		cmd := exec.Command(term)
		cmd.Dir = path
		return start(cmd)
	}
	for _, t := range terminais {
		if exe, err := exec.LookPath(t.exe); err == nil {
			cmd := exec.Command(exe, t.dirFlag+"="+path)
			cmd.Dir = path
			return start(cmd)
		}
	}
	return start(exec.Command("x-terminal-emulator"))
}

// readUserPath pergunta ao shell de login do usuário o PATH dele: o app
// aberto pela sessão gráfica pode não ter o mesmo PATH do terminal.
func readUserPath() (string, error) {
	return loginShell(`printf %s "$PATH"`)
}

// ensurePathsD: o HyPHP ainda não grava o PATH do sistema no Linux.
func ensurePathsD() error { return i18n.Errorf("err.linux.unavailable") }

// shadowingPHP pergunta ao shell de login qual php ele acha. Sem php nenhum,
// nada a avisar.
func shadowingPHP() (string, error) {
	out, err := loginShell("command -v php")
	if err != nil {
		return "", nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	return phpShadow(strings.TrimSpace(lines[len(lines)-1]), paths.Cli()), nil
}

// loginShell roda script no shell do usuário ($SHELL, ou /bin/sh) como shell
// de login, que lê os mesmos arquivos de perfil que o terminal.
func loginShell(script string) (string, error) {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err != nil {
		return "", fmt.Errorf("app: shell de login %s: %w", sh, err)
	}
	return string(out), nil
}
