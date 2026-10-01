package stack

import (
	"os"
	"path/filepath"
	"strings"
)

// pathCandidates lista os caminhos a testar para name em dir: o nome como veio
// e, se ele não tem extensão, cada extensão de PATHEXT — é assim que o cmd.exe
// acha `npm` como npm.cmd.
func pathCandidates(dir, name string) []string {
	cands := []string{filepath.Join(dir, name)}
	if filepath.Ext(name) != "" {
		return cands
	}
	// PATHEXT vem em maiúsculas (".COM;.EXE;.BAT"). O sistema de arquivos do
	// Windows é case-insensitive, mas o caminho vai para o Spec, aparece na
	// UI e entra na comparação do Reconcile: minúsculas mantêm o valor
	// estável e legível.
	for _, e := range strings.Split(pathExt(), ";") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			cands = append(cands, filepath.Join(dir, name+e))
		}
	}
	return cands
}

func pathExt() string {
	if v := os.Getenv("PATHEXT"); v != "" {
		return v
	}
	return ".COM;.EXE;.BAT;.CMD"
}

// isExecFile no Windows só exige arquivo existente: a executabilidade vem da
// extensão, já tratada em pathCandidates.
func isExecFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
