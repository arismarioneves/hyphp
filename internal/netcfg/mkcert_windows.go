package netcfg

import (
	"errors"
	"os"
	"path/filepath"
)

// CAInstalled devolve true se CARoot/rootCA.pem existe. Aproximação: o arquivo é criado por
// `mkcert -install`; a confiança na store do Windows é verificada no smoke com certutil.
func (m Mkcert) CAInstalled() (bool, error) {
	if m.CARoot == "" {
		return false, errors.New("CARoot não definido")
	}
	_, err := os.Stat(filepath.Join(m.CARoot, "rootCA.pem"))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
