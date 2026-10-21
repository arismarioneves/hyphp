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
// Code (`code` no PATH), como no Windows. Sem o programa, o erro diz onde
// escolher outro: o do exec só falaria em $PATH.
func (a *AppService) openInEditor(path string) error {
	editor := expandEnv(strings.TrimSpace(a.d.State().Editor))
	if editor == "" {
		if _, err := exec.LookPath("code"); err != nil {
			return i18n.Errorf("err.app.noEditor")
		}
		editor = "code"
	}
	if _, err := exec.LookPath(editor); err != nil {
		return i18n.Errorf("err.app.editorNotFound", editor)
	}
	return start(exec.Command(editor, path))
}

// terminais são os emuladores tentados quando o usuário não configurou um.
// x-terminal-emulator é o terminal preferido do Debian e derivados; os outros
// cobrem o GNOME (o Ptyxis do Ubuntu 25.10 em diante, o Terminal e o Console),
// o KDE e o Xfce.
var terminais = []string{"x-terminal-emulator", "ptyxis", "gnome-terminal", "kgx", "konsole", "xfce4-terminal"}

// terminalArgs é a opção que abre o terminal name já na pasta dir. Os que não
// estão aqui abrem no diretório de trabalho do processo, que já é dir.
func terminalArgs(name, dir string) []string {
	switch name {
	case "ptyxis":
		// Sem --new-window o Ptyxis só traz para a frente a janela que já
		// está aberta, na pasta dela.
		return []string{"--new-window", "--working-directory=" + dir}
	case "konsole":
		return []string{"--workdir", dir}
	case "gnome-terminal", "kgx", "xfce4-terminal", "mate-terminal", "tilix":
		return []string{"--working-directory=" + dir}
	}
	return nil
}

// terminalName é o terminal de verdade por trás de exe: o x-terminal-emulator
// é um link (pelas alternativas do Debian) para o preferido, e o do GNOME
// Terminal é o gnome-terminal.wrapper.
func terminalName(exe string) string {
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return strings.TrimSuffix(filepath.Base(exe), ".wrapper")
}

// openTerminal abre o terminal já na pasta: o configurado ou, vazio, o
// primeiro da lista que existir.
func (a *AppService) openTerminal(path string) error {
	if term := expandEnv(strings.TrimSpace(a.d.State().Terminal)); term != "" {
		exe, err := exec.LookPath(term)
		if err != nil {
			return i18n.Errorf("err.app.terminalNotFound", term)
		}
		cmd := exec.Command(exe, terminalArgs(terminalName(exe), path)...)
		cmd.Dir = path
		return start(cmd)
	}
	for _, t := range terminais {
		exe, err := exec.LookPath(t)
		if err != nil {
			continue
		}
		name := terminalName(exe)
		// O wrapper do GNOME Terminal traduz as opções do xterm, não as dele:
		// a pasta vai direto para o gnome-terminal.
		if real, err := exec.LookPath(name); err == nil {
			exe = real
		}
		cmd := exec.Command(exe, terminalArgs(name, path)...)
		cmd.Dir = path
		return start(cmd)
	}
	return i18n.Errorf("err.app.noTerminal", strings.Join(terminais, ", "))
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
