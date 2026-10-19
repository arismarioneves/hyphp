//go:build darwin || linux

package services

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"hyphp/internal/paths"
	"hyphp/internal/runtime"
)

// expandEnv aceita ~ e $VAR, a forma de caminho que se copia do terminal.
func expandEnv(s string) string {
	if s == "~" || strings.HasPrefix(s, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			s = filepath.Join(home, strings.TrimPrefix(s, "~"))
		}
	}
	return os.ExpandEnv(s)
}

// cliPathDir: quem entra no PATH é a pasta cli, com o atalho hyphp.
func cliPathDir(string) string { return paths.Cli() }

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

// phpShadow devolve found quando ele não é o php da pasta cli do HyPHP.
func phpShadow(found, cliDir string) string {
	if found == "" || strings.HasPrefix(found, cliDir+"/") {
		return ""
	}
	return found
}

// pathComDir é a mesma conta do Windows com a regra do Unix: separador ":"
// e comparação exata, porque o sistema de arquivos pode diferenciar
// maiúsculas. Info a usa para dizer se a CLI já está no PATH.
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
