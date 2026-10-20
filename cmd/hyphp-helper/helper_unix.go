//go:build darwin || linux

package main

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// validCliDir aceita só a pasta cli que o app criou. Ela entra no PATH de todo
// shell de login da máquina, então quebra de linha injetaria outra entrada;
// pasta de root seria uma pasta que o usuário não controla. Devolve o dono.
func validCliDir(dir string) (uid int, err error) {
	switch {
	case dir == "" || !filepath.IsAbs(dir) || filepath.Clean(dir) != dir:
		return 0, fmt.Errorf("--dir precisa ser absoluto e limpo: %q", dir)
	case strings.ContainsAny(dir, "\n\r"):
		return 0, fmt.Errorf("--dir com quebra de linha: %q", dir)
	case filepath.Base(dir) != "cli":
		return 0, fmt.Errorf("--dir precisa terminar em /cli: %q", dir)
	}
	st, err := os.Stat(dir)
	if err != nil {
		return 0, fmt.Errorf("--dir: %w", err)
	}
	if !st.IsDir() {
		return 0, fmt.Errorf("--dir não é pasta: %q", dir)
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("--dir: dono desconhecido: %q", dir)
	}
	if sys.Uid == 0 {
		return 0, fmt.Errorf("--dir pertence a root: %q", dir)
	}
	return int(sys.Uid), nil
}

// validCACert aceita só um rootCA.pem de CA: o helper roda como root, e
// confiar em qualquer arquivo indicado abriria a confiança do sistema para
// certificados de terceiros. Devolve o certificado lido: quem grava a CA em
// outro lugar grava ele, e não o arquivo relido, que o usuário pode ter
// trocado no meio ou que pode trazer outros certificados depois do primeiro.
func validCACert(path string) (*x509.Certificate, error) {
	if !filepath.IsAbs(path) || filepath.Base(path) != "rootCA.pem" {
		return nil, fmt.Errorf("--cert precisa ser um rootCA.pem com caminho absoluto: %q", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("--cert: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("--cert não é um certificado PEM: %q", path)
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("--cert: %w", err)
	}
	if !c.IsCA {
		return nil, fmt.Errorf("--cert não é de uma CA: %q", path)
	}
	return c, nil
}

// writeFileAtomic grava num temporário em tmpDir e renomeia: uma regra de DNS
// pela metade mandaria as consultas .test para lugar nenhum. tmpDir fica no
// mesmo sistema de arquivos do destino, para o rename seguir atômico, e fora
// de qualquer pasta que o sistema leia inteira: um resto ali depois de uma
// queda viraria configuração. O chmod explícito tira a dependência do umask
// do processo elevado: quem lê o arquivo nem sempre é root (o mDNSResponder e
// o path_helper no Mac, o resolved e o shell do usuário no Linux).
func writeFileAtomic(path string, data []byte, tmpDir string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(tmpDir, "."+filepath.Base(path)+".hyphp-tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// outsideDir é a pasta acima da do destino (ex.: /etc para /etc/resolver/test).
// O sistema lê inteiras as pastas de configuração (/etc/resolver e
// /etc/paths.d no Mac; resolved.conf.d, profile.d, sysctl.d e ca-certificates
// no Linux): um temporário dentro delas, depois de uma queda, viraria
// configuração, como uma regra do domínio test.hyphp-tmp.
func outsideDir(path string) string { return filepath.Dir(filepath.Dir(path)) }
