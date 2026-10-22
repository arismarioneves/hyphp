package update

import (
	"os"
	"path/filepath"
	"runtime"
)

// manifestFile é o nome do manifesto guardado ao lado do .deb baixado; a
// assinatura fica em manifestFile + ".sig".
const manifestFile = "latest.json"

// platformArtifact é o pacote do Linux no manifesto. Só sai pacote amd64: em
// outra arquitetura, e sem linux_amd64, Check trata como "em dia".
func platformArtifact(l Latest) *Artifact {
	if runtime.GOARCH != "amd64" {
		return nil
	}
	return l.LinuxAMD64
}

// saveManifest guarda em dir o manifesto e a assinatura, nos bytes que
// chegaram: o install-update do helper, como root, confere a assinatura de
// novo e só aceita o .deb que o manifesto descreve.
func saveManifest(dir string, body, sig []byte) error {
	if err := os.WriteFile(filepath.Join(dir, manifestFile), body, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, manifestFile+".sig"), sig, 0o644)
}
