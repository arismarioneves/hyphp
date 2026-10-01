package stack

import (
	"os"
	"path/filepath"
)

// pathCandidates no macOS é o próprio nome: não há PATHEXT, o shell acha o
// executável pelo nome exato.
func pathCandidates(dir, name string) []string {
	return []string{filepath.Join(dir, name)}
}

// isExecFile exige um bit de execução: sem ele o exec falharia com permissão
// negada e o PATH deve seguir para o próximo diretório, como o shell faz.
func isExecFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}
