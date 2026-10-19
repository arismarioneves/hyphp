package netcfg

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// CAInstalled no Mac exige as duas respostas que o `security` dá sem senha: o
// certificado no keychain do sistema e o sistema confiando nele. Só o
// verify-cert não basta: depois de o certificado sair do keychain ele seguiu
// dando 0 no spike de 2026-10-10, e o ca-pending nunca voltaria.
func (m Mkcert) CAInstalled() (bool, error) {
	if m.CARoot == "" {
		return false, errors.New("CARoot não definido")
	}
	root := filepath.Join(m.CARoot, "rootCA.pem")
	sha, err := CertSHA1(root)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	in, err := InSystemKeychain(sha)
	if err != nil || !in {
		return false, err
	}
	// verify-cert sai com 1 quando o sistema não confia (CSSMERR_TP_NOT_TRUSTED).
	return exec.Command("/usr/bin/security", "verify-cert", "-c", root).Run() == nil, nil
}
