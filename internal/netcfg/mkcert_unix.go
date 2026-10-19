//go:build darwin || linux

package netcfg

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CreateCA cria a CA local como o usuário, sem tocar em nenhuma store do
// sistema. Como root, o mkcert criaria a chave com dono root (-r--------) e o
// app não conseguiria emitir os certificados dos sites. TRUST_STORES=nss deixa
// só o Firefox e, no Linux, o Chrome (quando o certutil do nss existe); a
// confiança do sistema vem depois, pelo helper.
func (m Mkcert) CreateCA() error {
	cmd := exec.Command(m.Exe, "-install")
	cmd.Env = append(os.Environ(), "CAROOT="+m.CARoot, "TRUST_STORES=nss")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mkcert -install: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
