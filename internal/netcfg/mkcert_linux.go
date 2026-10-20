package netcfg

import (
	"bytes"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// CAAnchorPath é a cópia da CA do mkcert que o update-ca-certificates junta
// ao pacote de CAs do sistema (ele só lê arquivos .crt). Variável: os testes
// apontam para uma pasta temporária.
var CAAnchorPath = "/usr/local/share/ca-certificates/hyphp-mkcert.crt"

// CAInstalled diz se a âncora do sistema é a CA deste mkcert. Uma âncora de
// outra CA (o usuário recriou a CAROOT) conta como não instalada: os sites
// sairiam com certificados que o sistema não confia.
func (m Mkcert) CAInstalled() (bool, error) {
	if m.CARoot == "" {
		return false, nil
	}
	own, err := certDER(filepath.Join(m.CARoot, "rootCA.pem"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	anchor, err := certDER(CAAnchorPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return bytes.Equal(own, anchor), nil
}

// certDER lê o primeiro certificado PEM do arquivo.
func certDER(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%s: sem certificado PEM", path)
	}
	return block.Bytes, nil
}
