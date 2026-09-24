package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultExtensions é a lista habilitada quando state.PHPExtensions não tem entrada
// para a série. "zip" é estático no PHP 8.x Windows; se não houver php_zip.dll ele
// simplesmente não aparece em ListExtensions e o php.ini gerado não o referencia.
// "zip" NÃO entra: no PHP 8.x para Windows ela é estática (aparece em `php -n -m`,
// sem php_zip.dll em ext/), então `extension=zip` seria redundante e gera warning
// que, com display_errors=On, vaza no corpo da resposta.
var DefaultExtensions = []string{"curl", "fileinfo", "gd", "intl", "mbstring", "openssl", "pdo_mysql", "pdo_sqlite", "sqlite3", "exif", "soap", "sodium"}

type Extension struct {
	Name    string `json:"name"` // "pdo_mysql"
	File    string `json:"file"` // "php_pdo_mysql.dll"
	Enabled bool   `json:"enabled"`
}

// ListExtensions lê inst.Dir/ext/php_*.dll (ordem alfabética de os.ReadDir) e marca
// Enabled para os nomes presentes em enabled.
func ListExtensions(inst Installed, enabled []string) ([]Extension, error) {
	extDir := filepath.Join(inst.Dir, "ext")
	entries, err := os.ReadDir(extDir)
	if err != nil {
		return nil, fmt.Errorf("runtime: ler %s: %w", extDir, err)
	}
	on := make(map[string]bool, len(enabled))
	for _, n := range enabled {
		on[n] = true
	}
	var out []Extension
	for _, e := range entries {
		name := e.Name()
		lower := strings.ToLower(name)
		if e.IsDir() || !strings.HasPrefix(lower, "php_") || !strings.HasSuffix(lower, ".dll") {
			continue
		}
		ext := name[len("php_") : len(name)-len(".dll")]
		out = append(out, Extension{Name: ext, File: name, Enabled: on[ext]})
	}
	return out, nil
}
