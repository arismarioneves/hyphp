package stack

import (
	"os"
	"path/filepath"
	"testing"

	"hyphp/internal/netcfg"
)

// Contrato irmão do TestSyncHostsReportaSemElevar: sem a CA local instalada, o
// Reconcile reporta "ca-pending" e serve os sites em HTTP — nunca eleva. Quem
// ainda não tem a CA é exatamente o usuário novo, que receberia um UAC antes da
// primeira tela. Instalar mora em Stack.InstallCA, sob ação do usuário.
func TestTLSIssuerReportaCAPendente(t *testing.T) {
	s := &Stack{d: Deps{Mkcert: netcfg.Mkcert{Exe: "mkcert.exe", CARoot: t.TempDir()}}}

	issue, warns := s.tlsIssuer()
	if issue != nil {
		t.Error("issue != nil: sem CA não há como emitir certificado")
	}
	if len(warns) != 1 || warns[0].Code != "ca-pending" {
		t.Fatalf("warnings = %+v, quero um ca-pending", warns)
	}
}

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

// Sem o binário do mkcert o caso é outro: não há o que instalar, então o aviso
// é "tls-unavailable" e nenhum botão de UAC deve ser oferecido.
func TestTLSIssuerSemMkcert(t *testing.T) {
	s := &Stack{d: Deps{Mkcert: netcfg.Mkcert{}}}

	if _, warns := s.tlsIssuer(); len(warns) != 1 || warns[0].Code != "tls-unavailable" {
		t.Fatalf("warnings = %+v, quero um tls-unavailable", warns)
	}
}
