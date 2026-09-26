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

// Apagar o último projeto deixa o hosts com domínios que precisam sair; o
// aviso tem de dizer isso, e não "domínios ainda não estão no hosts: " vazio.
func TestHostsChangeDescreveOsDoisSentidos(t *testing.T) {
	casos := []struct {
		nome       string
		have, want []string
		contem     []string
		naoContem  []string
	}{
		{"só remover", []string{"velho.test"}, nil, []string{"remover velho.test"}, []string{"adicionar"}},
		{"só adicionar", nil, []string{"novo.test"}, []string{"adicionar novo.test"}, []string{"remover"}},
		{"trocar", []string{"a.test", "b.test"}, []string{"b.test", "c.test"}, []string{"adicionar c.test", "remover a.test"}, []string{"b.test"}},
		{"mesmo conjunto", []string{"a.test"}, []string{"a.test"}, []string{"reescrito"}, nil},
	}
	for _, c := range casos {
		msg := hostsChange(c.have, c.want)
		for _, s := range c.contem {
			if !strings.Contains(msg, s) {
				t.Errorf("%s: %q não contém %q", c.nome, msg, s)
			}
		}
		for _, s := range c.naoContem {
			if strings.Contains(msg, s) {
				t.Errorf("%s: %q não deveria conter %q", c.nome, msg, s)
			}
		}
	}
}
