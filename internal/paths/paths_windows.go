//go:build windows

package paths

import (
	"os"
	"path/filepath"
	"strings"
)

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

// defaultRoot no Windows: diretório do executável se gravável e fora das
// áreas do sistema, senão %LOCALAPPDATA%\HyPHP (ver o comentário de Root).
func defaultRoot() string {
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
