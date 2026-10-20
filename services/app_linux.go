package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"hyphp/internal/elevate"
	"hyphp/internal/i18n"
	"hyphp/internal/paths"
	"hyphp/internal/runtime"
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
// aberto pela sessão gráfica pode não ter o mesmo PATH do terminal. Vale a
// última linha: um perfil que imprime algo no login sai antes dela.
func readUserPath() (string, error) {
	out, err := loginShell(`printf '\n%s' "$PATH"`)
	if err != nil {
		return "", err
	}
	return out[strings.LastIndexByte(out, '\n')+1:], nil
}

// addCLIToUserPath põe o atalho hyphp na pasta cli e registra a pasta no
// /etc/profile.d (senha só na primeira vez).
func addCLIToUserPath(exe string) error {
	if err := relink(filepath.Join(paths.Cli(), "hyphp"), exe); err != nil {
		return err
	}
	return ensureProfileD()
}

// addPHPToUserPath aponta o php-bin para a pasta bin da série padrão. A partir
// daí RefreshPathLinks o reaponta quando a série muda, sem senha.
func addPHPToUserPath(inst runtime.Installed) error {
	if err := relink(filepath.Join(paths.Cli(), paths.PhpBin), filepath.Join(inst.Dir, "bin")); err != nil {
		return err
	}
	return ensureProfileD()
}

// ensureProfileD pede a senha só quando o /etc/profile.d/hyphp.sh ainda não
// tem a pasta cli deste usuário. Depois confere se o shell de login dele lê o
// arquivo: o bash e o dash leem pelo /etc/profile, o zsh e o fish não, e sem o
// aviso o botão pareceria não ter feito nada.
func ensureProfileD() error {
	if !paths.ProfileDRegistered() {
		helper, err := elevate.HelperPath()
		if err != nil {
			return i18n.Errorf("err.stack.helperUnavailable", err)
		}
		switch err := elevate.RunElevated(helper, []string{"profile-write", "--dir", paths.Cli()}); {
		case errors.Is(err, elevate.ErrElevationDenied):
			return errors.New(i18n.T("err.path.cancelled"))
		case err != nil:
			return fmt.Errorf("helper profile-write: %w", err)
		}
	}
	current, err := readUserPath()
	if err != nil {
		// Sem o PATH do shell não há como conferir; o arquivo está gravado.
		return nil
	}
	if _, missing := pathComDir(current, paths.Cli()); missing {
		return errors.New(i18n.T("err.path.shellNoProfile", paths.ProfileDFile, loginShellPath(), paths.ProfileDFile))
	}
	return nil
}

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

// loginShellPath é o shell do usuário ($SHELL, ou /bin/sh).
func loginShellPath() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// loginShell roda script no shell do usuário como shell de login, que lê os
// mesmos arquivos de perfil que o terminal.
func loginShell(script string) (string, error) {
	sh := loginShellPath()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, sh, "-lc", script).Output()
	if err != nil {
		return "", fmt.Errorf("app: shell de login %s: %w", sh, err)
	}
	return string(out), nil
}
