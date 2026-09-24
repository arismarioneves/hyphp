// Package paths resolve a raiz de runtime do HyPHP e seus subdiretórios.
//
// Root é $HYPHP_ROOT quando definido; senão, o diretório do executável se der
// para escrever nele; senão, %LOCALAPPDATA%\HyPHP.
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

// writable é variável para o teste poder simular um diretório protegido sem
// depender de permissões reais da máquina.
var writable = canWrite

// canWrite responde se dá para criar arquivo em dir. Testar de verdade é a
// única resposta confiável no Windows: a ACL efetiva depende de herança,
// virtualização e do token do processo, e os.Stat não diz nada sobre isso.
func canWrite(dir string) bool {
	f, err := os.CreateTemp(dir, ".hyphp-write-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

// Root retorna a raiz de runtime, sempre absoluta e limpa. Ordem (C1):
// $HYPHP_ROOT, diretório do executável se for gravável, senão
// %LOCALAPPDATA%\HyPHP.
//
// A sonda de escrita existe por causa da instalação em Program Files: lá o
// usuário comum não escreve, e sem o desvio o app falharia ao gerar a primeira
// config — com erro de permissão em vez de explicação.
func Root() string {
	if v := os.Getenv(EnvRoot); v != "" {
		if abs, err := filepath.Abs(v); err == nil {
			return abs
		}
		return filepath.Clean(v)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if writable(dir) {
			return dir
		}
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "HyPHP")
	}
	wd, _ := os.Getwd()
	return wd
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
