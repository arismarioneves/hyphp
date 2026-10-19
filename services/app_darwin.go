package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"hyphp/internal/elevate"
	"hyphp/internal/i18n"
	"hyphp/internal/paths"
)

// shellOpen entrega pasta e URL ao `open`, que escolhe o app padrão do
// sistema, como o ShellExecute faz no Windows.
func shellOpen(target string) error { return start(exec.Command("open", target)) }

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

// readUserPath no Mac lê o /etc/paths.d/hyphp: o app aberto pelo Finder não
// herda o PATH do shell, e é por esse arquivo que a pasta cli entra no PATH
// dos shells de login. Devolve as linhas no formato do PATH (":").
func readUserPath() (string, error) {
	raw, err := os.ReadFile(paths.PathsDFile)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var dirs []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			dirs = append(dirs, line)
		}
	}
	return strings.Join(dirs, ":"), nil
}

// ensurePathsD pede a senha só quando o /etc/paths.d/hyphp ainda não tem a
// pasta cli deste usuário.
func ensurePathsD() error {
	if paths.PathsDRegistered() {
		return nil
	}
	helper, err := elevate.HelperPath()
	if err != nil {
		return i18n.Errorf("err.stack.helperUnavailable", err)
	}
	switch err := elevate.RunElevated(helper, []string{"paths-write", "--dir", paths.Cli()}); {
	case errors.Is(err, elevate.ErrElevationDenied):
		return errors.New(i18n.T("err.path.cancelled"))
	case err != nil:
		return fmt.Errorf("helper paths-write: %w", err)
	}
	return nil
}

// shadowingPHP pergunta a um zsh de login (o que o Terminal abre: lê o
// /etc/zprofile com o path_helper e o ~/.zprofile com o brew shellenv) qual
// php ele acha. Sem php nenhum, nada a avisar.
func shadowingPHP() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/bin/zsh", "-lc", "command -v php").Output()
	if err != nil {
		return "", nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return phpShadow(strings.TrimSpace(lines[len(lines)-1]), paths.Cli()), nil
}
