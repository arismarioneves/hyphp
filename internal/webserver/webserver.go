// Package webserver define o contrato comum aos web servers plugáveis do HyPHP
// (Apache e nginx) e os tipos de entrada da renderização de configuração.
//
// Um web server ativo por vez (spec §6.7): ambos querem as portas 80/443. A
// troca é feita a quente por stack.SwitchWebServer.
package webserver

import (
	"strings"

	"hyphp/internal/state"
	"hyphp/internal/supervisor"
)

// Site é um projeto publicado pelo web server.
type Site struct {
	ID          string   // id do projeto (nome normalizado); vira o nome do arquivo de vhost
	Domain      string   // "acme.test"
	Aliases     []string // ["*.acme.test"] quando wildcard
	Docroot     string   // absoluto, com "/" como separador
	PoolName    string   // "php81" — casa com PHPPool.Name
	TLSCert     string   // "" = sem TLS
	TLSKey      string
	HasHtaccess bool
}

// PHPPool é o conjunto de workers php-cgi de uma série de PHP (spec §6.3).
type PHPPool struct {
	Name    string // "php81" (sem pontos: letras+dígitos)
	Version string // "8.1"
	Ports   []int  // [9000, 9001, 9002, 9003]
}

// Tool é uma ferramenta servida pelo próprio web server — hoje só o
// phpMyAdmin. Fica numa porta própria e em 127.0.0.1, nunca em "*": é uma
// ferramenta administrativa com acesso ao banco sem senha, e expor isso na
// rede local entregaria o banco a quem estiver no mesmo wi-fi.
//
// Não é um Site porque não tem domínio, nem TLS, nem .htaccess do usuário, e
// não deve aparecer na lista de projetos.
type Tool struct {
	Name     string // "phpmyadmin" — vira o nome do arquivo de config
	Port     int    // porta dedicada (state.PhpMyAdminPort)
	Docroot  string // absoluto, com "/" como separador
	PoolName string // pool da série compatível
}

// Ports são as portas de escuta globais (state.HTTPPort / state.HTTPSPort).
type Ports struct {
	HTTP  int
	HTTPS int
}

// WebServer é a fachada que o stack usa para renderizar, validar, executar e
// sondar o web server ativo.
//
// RELOCABILIDADE (requisito duro): a saída de Render NUNCA embute o diretório
// de configuração absoluto. Motivo: stack.Reconcile renderiza em
// etc/<web>.next, valida *lá dentro* e só promove para etc/<web> se o
// validador passar. Se um arquivo gerado carimbasse o etcDir, o `-t` rodando
// em `.next` leria os arquivos antigos de etc/<web> e aprovaria a config
// errada — o oposto do que a validação existe para garantir.
//
// Como cada implementação cumpre isso:
//   - Apache: Validate/Command passam -C "Define HYPHP_ETC <etcDir>" e o
//     httpd.conf referencia ${HYPHP_ETC}/pools.conf, ${HYPHP_ETC}/vhosts/*.conf.
//   - nginx: Validate/Command passam -p <etcDir> e o nginx.conf usa includes
//     relativos (upstreams.conf, sites/*.conf), que o nginx resolve contra -p.
//
// Caminhos que NÃO são o etcDir (diretório do runtime, docroots, logDir,
// certificados) podem e devem ser absolutos.
// PageDataFile é a lista de projetos da página de host sem projeto. O stack a
// grava em PageDataDir() a cada Reconcile, e o web server a entrega em
// /dados/projetos.json só para esta máquina.
const PageDataFile = "projetos.json"

type WebServer interface {
	Name() state.WebServerName
	// Render devolve caminho-relativo-ao-etcDir (sempre com "/") → conteúdo.
	// Determinístico: mesma entrada, mesmos bytes.
	Render(sites []Site, pools []PHPPool, ports Ports, logDir string, tool *Tool) (map[string][]byte, error)
	// Validate roda o validador nativo apontando para etcDir. Erro traz a
	// saída completa do validador.
	Validate(etcDir string) error
	// Command devolve o processo de foreground a ser supervisionado.
	Command(etcDir string) (exe string, args []string, dir string)
	// Probe é a prova de readiness do serviço web (spec §7.2).
	Probe(ports Ports) supervisor.Probe

	// PageDataDir é a pasta de PageDataFile, relativa ao etcDir e com "/".
	// O Render declara nela um .keep: o stack grava a lista fora do Render
	// (ela não é configuração e não reinicia o servidor), e o WriteFiles do
	// web server não pode apagá-la.
	PageDataDir() string
}

// PoolName converte uma série de PHP no nome do pool/upstream: "8.1" → "php81".
// Apache (balancer://<nome>) e nginx (upstream <nome>) só aceitam identificador
// sem ponto, por isso o ponto some em vez de virar outro separador.
func PoolName(version string) string {
	return "php" + strings.ReplaceAll(version, ".", "")
}
