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

// cliPathDir: no Mac quem entra no PATH é a pasta cli, com o atalho hyphp.
func cliPathDir(string) string { return paths.Cli() }

// addCLIToUserPath põe o atalho hyphp na pasta cli e registra a pasta no
// /etc/paths.d (senha só na primeira vez).
func addCLIToUserPath(exe string) error {
	if err := relink(filepath.Join(paths.Cli(), "hyphp"), exe); err != nil {
		return err
	}
	return ensurePathsD()
}

// addPHPToUserPath aponta o php-bin para a pasta bin da série padrão. A partir
// daí RefreshPathLinks o reaponta quando a série muda, sem senha.
func addPHPToUserPath(inst runtime.Installed) error {
	if err := relink(filepath.Join(paths.Cli(), paths.PhpBin), filepath.Join(inst.Dir, "bin")); err != nil {
		return err
	}
	return ensurePathsD()
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

// relink aponta link para target, criando a pasta cli. Grava um link
// temporário e renomeia por cima: um terminal aberto no meio nunca vê o
// atalho faltando.
func relink(link, target string) error {
	if cur, err := os.Readlink(link); err == nil && cur == target {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	tmp := link + ".hyphp-tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, link)
}

// RefreshPathLinks refaz os atalhos da pasta cli ao abrir e quando os
// runtimes ou a série padrão mudam. Sem a pasta (o usuário nunca pediu PATH),
// nada; sem o php-bin (não pediu o PHP), só o hyphp.
func RefreshPathLinks(cliExe string, defaultPHP func() (runtime.Installed, bool)) error {
	dir := paths.Cli()
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := relink(filepath.Join(dir, "hyphp"), cliExe); err != nil {
		return err
	}
	bin := filepath.Join(dir, paths.PhpBin)
	if _, err := os.Lstat(bin); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	inst, ok := defaultPHP()
	if !ok {
		// Sem PHP nenhum o atalho fica como está: apagá-lo perderia a escolha
		// do usuário, e um PHP instalado depois volta a ser apontado.
		return nil
	}
	return relink(bin, filepath.Join(inst.Dir, "bin"))
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

// phpShadow devolve found quando ele não é o php da pasta cli do HyPHP.
func phpShadow(found, cliDir string) string {
	if found == "" || strings.HasPrefix(found, cliDir+"/") {
		return ""
	}
	return found
}

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
