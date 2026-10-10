package stack

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"hyphp/internal/webserver"
)

// O que a página mostra de cada projeto: se o domínio abre nesta máquina, o
// que falta quando não abre e se aceita subdomínios. Nada de pasta nem de
// certificado: o arquivo é servido pelo web server.
func TestProjetosDaPagina(t *testing.T) {
	sites := []webserver.Site{
		{ID: "multi", Domain: "multi.test", Aliases: []string{"*.multi.test"}, Docroot: "C:/DEV/multi/public"},
		{ID: "loja", Domain: "loja.test", Docroot: "C:/DEV/loja", TLSCert: "C:/HyPHP/var/certs/loja.pem", TLSKey: "C:/HyPHP/var/certs/loja-key.pem"},
	}
	casos := []struct {
		nome   string
		hosts  []string
		dns    bool
		motivo string
		quero  []paginaProjeto
	}{
		{"só o hosts (Windows sem a regra de DNS)", []string{"loja.test"}, false, "hosts", []paginaProjeto{
			{Domain: "loja.test", HTTPS: true, Abre: true},
			{Domain: "multi.test", Wildcard: true, Motivo: "hosts"},
		}},
		{"regra de DNS ativa", nil, true, "dns", []paginaProjeto{
			{Domain: "loja.test", HTTPS: true, Abre: true},
			{Domain: "multi.test", Wildcard: true, Abre: true, Subdominios: true},
		}},
		{"sem a regra de DNS (Mac)", nil, false, "dns", []paginaProjeto{
			{Domain: "loja.test", HTTPS: true, Motivo: "dns"},
			{Domain: "multi.test", Wildcard: true, Motivo: "dns"},
		}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			raw := projetosJSON(sites, webserver.Ports{HTTP: 8080, HTTPS: 8443}, c.hosts, c.dns, c.motivo)
			var got paginaDados
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if got.Portas != (paginaPortas{HTTP: 8080, HTTPS: 8443}) || !reflect.DeepEqual(got.Projetos, c.quero) {
				t.Errorf("projetos.json = %+v", got)
			}
			if strings.Contains(string(raw), "DEV") || strings.Contains(string(raw), "certs") {
				t.Errorf("caminho no projetos.json:\n%s", raw)
			}
		})
	}
	// Sem projeto, a lista é vazia, não null: o script percorre a lista sem
	// conferir.
	if raw := projetosJSON(nil, webserver.Ports{HTTP: 80, HTTPS: 443}, nil, false, "dns"); !strings.Contains(string(raw), `"projetos": []`) {
		t.Errorf("sem projetos:\n%s", raw)
	}
}
