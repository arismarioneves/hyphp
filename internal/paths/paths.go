// Package paths resolve a raiz de runtime do HyPHP e seus subdiretórios.
//
// Root é $HYPHP_ROOT quando definido; senão, o diretório do executável.
// Em `wails3 dev` o executável fica em bin/, então o Taskfile define
// HYPHP_ROOT=<repo>/.runtime.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnvRoot é a variável de ambiente que sobrescreve a raiz de runtime.
const EnvRoot = "HYPHP_ROOT"

// Root retorna a raiz de runtime, sempre absoluta e limpa.
func Root() string {
	if v := os.Getenv(EnvRoot); v != "" {
		if abs, err := filepath.Abs(v); err == nil {
			return abs
		}
		return filepath.Clean(v)
	}
	exe, err := os.Executable()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	return filepath.Dir(exe)
}

// Bin é Root()/bin — runtimes instalados (php/, apache/, nginx/, mysql/, mailpit/, mkcert/).
func Bin() string { return filepath.Join(Root(), "bin") }

// Etc é Root()/etc — configs geradas (descartáveis).
func Etc() string { return filepath.Join(Root(), "etc") }

// Var é Root()/var — state.json, mysql-data/, certs/.
func Var() string { return filepath.Join(Root(), "var") }

// Log é Root()/log — logs de processos supervisionados.
func Log() string { return filepath.Join(Root(), "log") }

// Run é Root()/var/run — pids e sockets efêmeros.
func Run() string { return filepath.Join(Var(), "run") }

// EnsureLayout cria bin/, etc/, var/, var/run/ e log/ (0755). Idempotente.
func EnsureLayout() error {
	for _, dir := range []string{Bin(), Etc(), Var(), Run(), Log()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("paths: criar %s: %w", dir, err)
		}
	}
	return nil
}
