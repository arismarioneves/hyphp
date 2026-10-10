package netcfg

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// SystemKeychain é onde a CA local fica confiável para todo o macOS (Safari,
// Chrome, curl do sistema).
const SystemKeychain = "/Library/Keychains/System.keychain"

// CertSHA1 devolve o SHA-1 do primeiro certificado PEM do arquivo, em hex
// maiúsculo, o formato que o `security` usa em -Z.
func CertSHA1(pemPath string) (string, error) {
	raw, err := os.ReadFile(pemPath)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("%s: sem certificado PEM", pemPath)
	}
	sum := sha1.Sum(block.Bytes)
	return strings.ToUpper(hex.EncodeToString(sum[:])), nil
}

// InSystemKeychain diz se o certificado com esse SHA-1 está no keychain do
// sistema. Ler o keychain não pede senha.
func InSystemKeychain(sha1Hex string) (bool, error) {
	out, err := exec.Command("/usr/bin/security", "find-certificate", "-a", "-Z", SystemKeychain).Output()
	if err != nil {
		return false, fmt.Errorf("security find-certificate: %w", err)
	}
	return keychainHasSHA1(string(out), sha1Hex), nil
}

// keychainHasSHA1 procura a linha "SHA-1 hash: <hex>" da saída do
// `security find-certificate -Z`.
func keychainHasSHA1(out, sha1Hex string) bool {
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "SHA-1 hash:"); ok && strings.EqualFold(strings.TrimSpace(v), sha1Hex) {
			return true
		}
	}
	return false
}
