package stack

import (
	"strings"
	"testing"

	"hyphp/internal/webserver"
)

// Contrato da correção: o Reconcile NUNCA eleva por causa do hosts. Pedir UAC
// no caminho do boot travava o app antes da primeira tela e empilhava um
// diálogo por Reconcile. A escrita mora em ApplyHosts, sob ação do usuário.
//
// O teste usa um domínio que não tem como estar no hosts da máquina, então a
// pendência é determinística: se syncHosts fosse voltar a elevar, este teste
// travaria esperando UAC em vez de retornar.
func TestSyncHostsReportaSemElevar(t *testing.T) {
	sites := []webserver.Site{
		{Domain: "zz-hyphp-inexistente.test"},
		{Domain: "zz-hyphp-outro.test"},
	}

	warns, err := new(Stack).syncHosts(sites)
	if err != nil {
		t.Fatalf("syncHosts: %v", err)
	}
	if len(warns) != 1 {
		t.Fatalf("esperava 1 warning, veio %d: %+v", len(warns), warns)
	}
	if warns[0].Code != "hosts-pending" {
		t.Errorf("Code = %q, quero hosts-pending", warns[0].Code)
	}
	for _, s := range sites {
		if !strings.Contains(warns[0].Message, s.Domain) {
			t.Errorf("mensagem não cita %q: %s", s.Domain, warns[0].Message)
		}
	}
}
