package runtime

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
)

// DefaultExtensions é a lista habilitada quando state.PHPExtensions não tem entrada
// para a série. "zip" NÃO entra: no PHP 8.x para Windows ela é estática (aparece
// em `php -n -m`, sem php_zip.dll em ext/, e ListExtensions a mostra como
// embutida), então `extension=zip` seria redundante e gera warning que, com
// display_errors=On, vaza no corpo da resposta. No Homebrew os nomes da lista
// que vêm compilados aparecem como embutidos, e o render os ignora por falta
// de arquivo.
var DefaultExtensions = []string{"curl", "fileinfo", "gd", "intl", "mbstring", "mysqli", "openssl", "pdo_mysql", "pdo_sqlite", "sqlite3", "exif", "soap", "sodium"}

type Extension struct {
	Name    string `json:"name"` // "pdo_mysql"
	File    string `json:"file"` // "php_pdo_mysql.dll"; vazio nos embutidos
	Enabled bool   `json:"enabled"`
	// Builtin marca o módulo compilado no binário (`php -n -m`): está sempre
	// ligado e não há o que alternar.
	Builtin bool `json:"builtin"`
}

// ListExtensions junta os módulos embutidos (builtin, de BuiltinModules) com
// os arquivos de inst.ExtDir (regra de extName), em ordem de nome. Embutido sai
// Enabled e Builtin; arquivo sai Enabled se o nome está em enabled. Um nome que
// é embutido e também tem arquivo fica só como embutido: carregar o arquivo de
// novo daria o warning "already loaded". Pasta inexistente não é erro: o PHP
// do Homebrew pode não ter módulo carregável algum.
func ListExtensions(inst Installed, enabled, builtin []string) ([]Extension, error) {
	entries, err := os.ReadDir(inst.ExtDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("runtime: ler %s: %w", inst.ExtDir, err)
	}
	out := make([]Extension, 0, len(builtin)+len(entries))
	for _, n := range builtin {
		out = append(out, Extension{Name: n, Enabled: true, Builtin: true})
	}
	for _, e := range entries {
		name := e.Name()
		ext, ok := extName(name)
		if e.IsDir() || !ok || slices.Contains(builtin, ext) {
			continue
		}
		out = append(out, Extension{Name: ext, File: name, Enabled: slices.Contains(enabled, ext)})
	}
	slices.SortStableFunc(out, func(a, b Extension) int { return cmp.Compare(a.Name, b.Name) })
	return out, nil
}

// BuiltinModules lista os módulos compilados no binário: `php -n -m` sem
// php.ini não carrega nada de ExtDir, então tudo o que aparece é estático (no
// Windows o zip, no 8.5 o opcache).
func BuiltinModules(ctx context.Context, inst Installed) ([]string, error) {
	out, err := run(ctx, inst.Dir, inst.Exe, "-n", "-m")
	if err != nil {
		return nil, err
	}
	return parseModules(out), nil
}

// parseModules lê a saída de `php -m`: pula os cabeçalhos "[PHP Modules]" e
// "[Zend Modules]", põe os nomes em minúsculas (a grafia de extension=) e
// troca "Zend OPcache" por "opcache". Sem repetição: o OPcache aparece nas
// duas seções.
func parseModules(out string) []string {
	var mods []string
	for line := range strings.Lines(out) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[") {
			continue
		}
		name := strings.ToLower(line)
		if name == "zend opcache" {
			name = "opcache"
		}
		if !slices.Contains(mods, name) {
			mods = append(mods, name)
		}
	}
	return mods
}
