package netcfg

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"hyphp/internal/sysproc"
)

// renewBefore: certificados que expiram em menos que isto são reemitidos (spec §10.4).
const renewBefore = 30 * 24 * time.Hour

// Mkcert encapsula o mkcert.exe. CARoot é a saída de `mkcert -CAROOT`; CertDir é var/certs.
type Mkcert struct{ Exe, CARoot, CertDir string }

// NewMkcert roda `mkcert -CAROOT` uma vez e devolve o Mkcert com CARoot preenchido.
func NewMkcert(exe, certDir string) (Mkcert, error) {
	cmd := exec.Command(exe, "-CAROOT")
	sysproc.Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return Mkcert{}, fmt.Errorf("mkcert -CAROOT: %w", err)
	}
	caroot := strings.TrimSpace(string(out))
	if caroot == "" {
		return Mkcert{}, errors.New("mkcert -CAROOT devolveu vazio")
	}
	return Mkcert{Exe: exe, CARoot: caroot, CertDir: certDir}, nil
}

// IssueCert emite (ou reutiliza) um certificado para o conjunto de domínios. O nome do arquivo
// é derivado do hash dos domínios ordenados, então a mesma lista sempre cai no mesmo par
// <hash>.pem / <hash>-key.pem. Reemite se o cert não existir, não parsear, ou expirar em < 30 dias.
func (m Mkcert) IssueCert(domains []string) (certPath, keyPath string, err error) {
	domains = normalizeDomains(domains)
	if len(domains) == 0 {
		return "", "", errors.New("IssueCert: nenhum domínio")
	}
	base := certBaseName(domains)
	certPath = filepath.Join(m.CertDir, base+".pem")
	keyPath = filepath.Join(m.CertDir, base+"-key.pem")

	if pemBytes, readErr := os.ReadFile(certPath); readErr == nil {
		if _, keyErr := os.Stat(keyPath); keyErr == nil && !needsRenewal(pemBytes, time.Now()) {
			return certPath, keyPath, nil
		}
	}

	if err := os.MkdirAll(m.CertDir, 0o755); err != nil {
		return "", "", fmt.Errorf("IssueCert: %w", err)
	}
	args := append([]string{"-cert-file", certPath, "-key-file", keyPath}, domains...)
	cmd := exec.Command(m.Exe, args...)
	sysproc.Hide(cmd)
	if m.CARoot != "" {
		cmd.Env = append(os.Environ(), "CAROOT="+m.CARoot)
	}
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		return "", "", fmt.Errorf("mkcert %s: %w: %s", strings.Join(domains, " "), runErr, strings.TrimSpace(string(out)))
	}
	if _, statErr := os.Stat(certPath); statErr != nil {
		return "", "", fmt.Errorf("mkcert não gerou %s: %w", certPath, statErr)
	}
	return certPath, keyPath, nil
}

// certBaseName devolve os 12 primeiros hex do SHA-256 dos domínios (já normalizados) unidos por '\n'.
func certBaseName(domains []string) string {
	sum := sha256.Sum256([]byte(strings.Join(domains, "\n")))
	return hex.EncodeToString(sum[:])[:12]
}

// needsRenewal decide pela reemissão: PEM inválido, não-certificado, ou NotAfter < now+30d.
func needsRenewal(certPEM []byte, now time.Time) bool {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return true
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return true
	}
	return cert.NotAfter.Before(now.Add(renewBefore))
}
