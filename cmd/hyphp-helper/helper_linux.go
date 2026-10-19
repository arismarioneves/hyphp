// Command hyphp-helper é o binário do HyPHP que roda com privilégio de
// administrador. No Linux ele ainda não grava nada: o app não pede senha
// aqui, e uma chamada direta sai com código 2.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "hyphp-helper: not available on Linux yet")
	os.Exit(2)
}
