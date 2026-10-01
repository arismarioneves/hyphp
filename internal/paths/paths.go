// Package paths resolve a raiz de runtime do HyPHP e seus subdiretórios.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnvRoot é a variável de ambiente que sobrescreve a raiz de runtime.
const EnvRoot = "HYPHP_ROOT"

// Root retorna a raiz de runtime, sempre absoluta e limpa.
//
// Ordem: $HYPHP_ROOT; senão defaultRoot(), que é por SO. No Windows: diretório
// do executável, se der para escrever nele e não for área do sistema; senão
// %LOCALAPPDATA%\HyPHP. No macOS: ~/Library/Application Support/HyPHP.
//
// A recusa de Program Files, Program Files (x86) e Windows é o que mantém a
// resposta estável: esses diretórios são graváveis para um processo elevado, e
// com só a sonda de escrita abrir o app como administrador mudava a raiz para
// o diretório de instalação — o usuário via projetos, runtimes e configurações
// desaparecerem, porque havia dois ambientes escolhidos pelo privilégio.
//
// Instalado fora dessas áreas (C:\HyPHP, por exemplo), a raiz é a própria
// pasta da instalação. Preferir %LOCALAPPDATA% nesse caso, por causa de uma
// instalação anterior, criaria o problema inverso: o usuário instala runtimes
// numa raiz e o app lê a outra, vendo tudo vazio.
func Root() string {
	if frozen != "" {
		return frozen
	}
	return resolveRoot()
}

// frozen é a raiz fixada por Freeze; vazio = recalcular a cada chamada.
var frozen string

// Freeze calcula Root() uma vez e a fixa para o resto do processo. Sem isso
// cada Etc()/Var()/Log() refazia a sonda de escrita, e uma falha passageira
// dela (antivírus segurando a pasta, disco cheio) mandava só aquela chamada
// para %LOCALAPPDATA%: state.json numa raiz, etc/ e logs em outra. O app
// chama no início do main, antes de qualquer goroutine; os testes não chamam
// e continuam variando HYPHP_ROOT à vontade.
func Freeze() {
	frozen = resolveRoot()
}

func resolveRoot() string {
	if v := os.Getenv(EnvRoot); v != "" {
		if abs, err := filepath.Abs(v); err == nil {
			return abs
		}
		return filepath.Clean(v)
	}
	return defaultRoot()
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
