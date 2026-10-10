package stack

import (
	"os"
	"path/filepath"
	"testing"

	"hyphp/internal/netcfg"
)

// Com a CA presente o emissor aparece e nenhum aviso é gerado — senão a UI
// ofereceria "instalar certificado" para sempre.
func TestTLSIssuerComCAInstalada(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rootCA.pem"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Stack{d: Deps{Mkcert: netcfg.Mkcert{Exe: "mkcert.exe", CARoot: root}}}

	issue, warns := s.tlsIssuer()
	if issue == nil {
		t.Error("issue == nil com a CA instalada")
	}
	if len(warns) != 0 {
		t.Errorf("warnings = %+v, quero nenhum", warns)
	}
}
