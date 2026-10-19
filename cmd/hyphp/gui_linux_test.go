package main

import "testing"

// O pacote e o build local têm o mesmo miolo: a CLI em cli/, o app uma pasta
// acima.
func TestAppFicaAoLadoDaPastaDaCLI(t *testing.T) {
	casos := map[string]string{
		"/usr/lib/hyphp/cli/hyphp":      "/usr/lib/hyphp/hyphp",
		"/home/dev/hyphp/bin/cli/hyphp": "/home/dev/hyphp/bin/hyphp",
	}
	for cli, quer := range casos {
		if got := appBeside(cli); got != quer {
			t.Errorf("appBeside(%q) = %q; quero %q", cli, got, quer)
		}
	}
}
