package brew

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"hyphp/internal/pkgmgr"
)

// O brew sai com 1 quando só o link em <prefix>/bin conflita com outra
// fórmula (mariadb@11.8 ao lado do mysql@8.4), mas deixa o keg instalado e o
// opt/ apontado: para o HyPHP, que só usa o opt/, a instalação deu certo. O
// mesmo código de saída sem keg no opt/ continua sendo falha.
func TestInstallComLinkEmConflito(t *testing.T) {
	for _, c := range []struct {
		nome     string
		criaKeg  bool
		querErro bool
	}{
		{"keg no opt e link recusado", true, false},
		{"falha sem keg no opt", false, true},
	} {
		t.Run(c.nome, func(t *testing.T) {
			prefix := t.TempDir()
			script := "#!/bin/sh\n"
			if c.criaKeg {
				script += fmt.Sprintf("mkdir -p '%[1]s/Cellar/mariadb@11.8/11.8.3' '%[1]s/opt' && "+
					"ln -s ../Cellar/mariadb@11.8/11.8.3 '%[1]s/opt/mariadb@11.8'\n", prefix)
			}
			script += "echo 'Error: The `brew link` step did not complete successfully'\nexit 1\n"
			exe := filepath.Join(t.TempDir(), "brew")
			if err := os.WriteFile(exe, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}

			err := Brew{Exe: exe, Prefix: prefix}.Install(context.Background(),
				Formula{Name: "mariadb@11.8"}, func(pkgmgr.Progress) {})
			if got := err != nil; got != c.querErro {
				t.Fatalf("Install: err = %v, quer erro = %v", err, c.querErro)
			}
		})
	}
}
