package stack

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"

	"hyphp/internal/paths"
	"hyphp/internal/render"
	"hyphp/internal/state"
	"hyphp/internal/webserver"
)

// paginaProjeto é um projeto como a página de host sem projeto o mostra. O
// arquivo é servido pelo web server: só o domínio e o estado, nunca pasta ou
// caminho.
type paginaProjeto struct {
	Domain   string `json:"domain"`
	Wildcard bool   `json:"wildcard"`
	HTTPS    bool   `json:"https"`
	// Abre diz se o domínio resolve nesta máquina hoje; Motivo, quando não
	// abre, é o que falta: "hosts" (Windows) ou "dns".
	Abre        bool   `json:"abre"`
	Motivo      string `json:"motivo,omitempty"`
	Subdominios bool   `json:"subdominios"`
}

type paginaPortas struct {
	HTTP  int `json:"http"`
	HTTPS int `json:"https"`
}

type paginaDados struct {
	Portas   paginaPortas    `json:"portas"`
	Projetos []paginaProjeto `json:"projetos"`
}

// projetosJSON monta a lista da página. hosts são os domínios do bloco do
// hosts aplicado (vazio no Mac); dns diz se a regra do .test leva a esta
// máquina; motivo é o que falta a um domínio que não abre. Wildcard vem dos
// aliases porque o desired só cria o alias *.<domínio> para projeto com
// wildcard.
func projetosJSON(sites []webserver.Site, ports webserver.Ports, hosts []string, dns bool, motivo string) []byte {
	d := paginaDados{
		Portas:   paginaPortas{HTTP: ports.HTTP, HTTPS: ports.HTTPS},
		Projetos: make([]paginaProjeto, 0, len(sites)),
	}
	for _, site := range sites {
		p := paginaProjeto{
			Domain:   site.Domain,
			Wildcard: len(site.Aliases) > 0,
			HTTPS:    site.TLSCert != "",
			Abre:     dns || slices.Contains(hosts, site.Domain),
		}
		p.Subdominios = p.Wildcard && dns
		if !p.Abre {
			p.Motivo = motivo
		}
		d.Projetos = append(d.Projetos, p)
	}
	slices.SortFunc(d.Projetos, func(a, b paginaProjeto) int { return strings.Compare(a.Domain, b.Domain) })
	raw, _ := json.MarshalIndent(d, "", "  ")
	return append(raw, '\n')
}

// writePageData grava a lista da página de host sem projeto. Fica fora do
// renderWeb de propósito: a lista muda a cada projeto e não é configuração, e
// passar pelo render reiniciaria o Apache ou o nginx a cada mudança. Uma
// falha só vai para o log: sem a lista, a página mostra a mensagem genérica.
func (s *Stack) writePageData(web webserver.WebServer, sites []webserver.Site, st state.State) {
	hosts, dns, motivo := s.resolucao()
	raw := projetosJSON(sites, webserver.Ports{HTTP: st.HTTPPort, HTTPS: st.HTTPSPort}, hosts, dns, motivo)
	dir := filepath.Join(paths.Etc(), string(web.Name()), filepath.FromSlash(web.PageDataDir()))
	// O .keep no mapa impede este WriteFiles de apagar o que o do web server
	// criou na pasta.
	if _, err := render.WriteFiles(dir, map[string][]byte{render.KeepFile: nil, webserver.PageDataFile: raw}); err != nil {
		s.d.Logger.Warn("stack: gravar a lista de projetos da página", "err", err)
	}
}
