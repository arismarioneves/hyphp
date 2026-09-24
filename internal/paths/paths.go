// Package paths resolve a raiz de runtime do HyPHP e seus subdiretórios.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// localRoot é %LOCALAPPDATA%\HyPHP ("" se a variável não existir).
func localRoot() string {
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "HyPHP")
	}
	return ""
}

// protegido responde se dir está sob um diretório do sistema onde dados de
// usuário não devem morar, mesmo que o processo consiga escrever lá.
func protegido(dir string) bool {
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "SystemRoot"} {
		base := os.Getenv(env)
		if base == "" {
			continue
		}
		rel, err := filepath.Rel(base, dir)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// Root retorna a raiz de runtime, sempre absoluta e limpa.
//
// Ordem: $HYPHP_ROOT; raiz já existente em %LOCALAPPDATA%\HyPHP; diretório do
// executável, se for gravável e não estiver sob Program Files ou Windows;
// senão %LOCALAPPDATA%\HyPHP.
//
// As duas primeiras regras existem porque a resposta NÃO pode depender do
// token do processo. Program Files é gravável para um processo elevado: com só
// a sonda de escrita, abrir o app como administrador mudava a raiz para o
// diretório de instalação e o usuário via projetos, runtimes e configurações
// desaparecerem — havia dois ambientes, escolhidos pelo nível de privilégio.
func Root() string {
	if v := os.Getenv(EnvRoot); v != "" {
		if abs, err := filepath.Abs(v); err == nil {
			return abs
		}
		return filepath.Clean(v)
	}
	// Raiz já usada antes vence: uma vez que os dados moram em LOCALAPPDATA,
	// nenhuma mudança de contexto pode apontar para outro lugar.
	if local := localRoot(); local != "" {
		if _, err := os.Stat(filepath.Join(local, "var", "state.json")); err == nil {
			return local
		}
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if !protegido(dir) && writable(dir) {
			return dir
		}
	}
	if local := localRoot(); local != "" {
		return local
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
