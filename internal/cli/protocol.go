// Package cli é o contrato entre a CLI (cmd/hyphp) e o app aberto: o
// transporte (named pipe do usuário), as rotas, os tipos que trafegam e a
// tabela de comandos que alimenta a ajuda da CLI e a aba CLI do app.
//
// O app é quem executa: cada comando vira uma chamada aos mesmos serviços que
// a UI usa. Os tipos daqui são o formato publicado para scripts e agentes de
// IA (a saída --json da CLI); campo existente não muda de nome nem de sentido.
package cli

// Rotas. POST /v1/call/<comando> leva os argumentos em JSON e devolve o
// resultado em JSON (200) ou {"error": "..."} (4xx/5xx). POST /v1/logs devolve
// texto em streaming, uma linha de log por linha.
const (
	CallPath = "/v1/call/"
	LogsPath = "/v1/logs"
)

// Cabeçalhos. Lang vai na resposta: o idioma do app, que a CLI usa no próprio
// texto. Caller e Argv vão no pedido: o executável que chamou a CLI
// (pwsh.exe, claude.exe…) e o que foi digitado depois de `hyphp`, mostrados
// na atividade da aba CLI.
const (
	HeaderLang   = "X-HyPHP-Lang"
	HeaderCaller = "X-HyPHP-Caller"
	HeaderArgv   = "X-HyPHP-Argv"
)

// Comandos do app (o que vem depois de /v1/call/).
const (
	CmdStatus     = "status"
	CmdServices   = "services"
	CmdStart      = "start"
	CmdStop       = "stop"
	CmdRestart    = "restart"
	CmdProjects   = "projects"
	CmdPHP        = "php"
	CmdPHPDefault = "php.default"
	CmdPHPUse     = "php.use"
	CmdIni        = "ini"
	CmdIniSet     = "ini.set"
	CmdIniReset   = "ini.reset"
	CmdDB         = "db"
	CmdDBCreate   = "db.create"
	CmdDBDrop     = "db.drop"
	CmdDBEngine   = "db.engine"
	CmdWeb        = "web"
	CmdWarnings   = "warnings"
	CmdShowWindow = "app.show"
)

// AllServices é o argumento de start/stop que vale para todos os serviços.
const AllServices = "all"

// ErrorBody é o corpo de uma resposta de erro.
type ErrorBody struct {
	Error string `json:"error"`
}

// ---- argumentos ----

// ServiceArgs: start, stop, restart. ID vazio ou "all" em start/stop vale
// para todos.
type ServiceArgs struct {
	ID string `json:"id"`
}

// PHPDefaultArgs: php.default.
type PHPDefaultArgs struct {
	Major string `json:"major"`
}

// PHPUseArgs: php.use. Project é o id do projeto (o nome da pasta).
type PHPUseArgs struct {
	Project string `json:"project"`
	Major   string `json:"major"`
}

// IniArgs: ini, ini.set, ini.reset. Name e Value só onde fazem sentido.
type IniArgs struct {
	Major string `json:"major"`
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// NameArgs: db.create, db.drop, db.engine, web.
type NameArgs struct {
	Name string `json:"name"`
}

// LogsArgs: /v1/logs. Lines ≤ 0 vale o padrão do app; Follow mantém a
// resposta aberta com as linhas novas até o cliente desistir.
type LogsArgs struct {
	ID     string `json:"id"`
	Lines  int    `json:"lines"`
	Follow bool   `json:"follow"`
}

// ---- resultados ----

// Status é o resumo do app.
type Status struct {
	Version    string `json:"version"`
	Root       string `json:"root"`
	Language   string `json:"language"`
	WebServer  string `json:"webServer"`
	DBEngine   string `json:"dbEngine"`
	DefaultPHP string `json:"defaultPhp"`
	Ready      int    `json:"ready"`
	Total      int    `json:"total"`
	Failed     int    `json:"failed"`
	Warnings   int    `json:"warnings"`
}

// Service é um processo supervisionado.
type Service struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Group     string `json:"group"`
	State     string `json:"state"`
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	Restarts  int    `json:"restarts"`
	LastError string `json:"lastError"`
}

// Project é um projeto descoberto nas pastas do usuário.
type Project struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	PHP      string `json:"php"` // série efetiva (manifesto → padrão → maior instalada)
	Root     string `json:"root"`
	Docroot  string `json:"docroot"`
	Database string `json:"database"`
	Manifest bool   `json:"manifest"` // tem hyphp.yaml
}

// PHP é uma versão de PHP instalada.
type PHP struct {
	Version string `json:"version"`
	Major   string `json:"major"`
	Dir     string `json:"dir"`
	Default bool   `json:"default"`
}

// IniSetting é uma diretiva do php.ini de uma série. Source: "hyphp" (padrão
// do HyPHP), "php" (padrão do PHP) ou "user" (definida pelo usuário).
type IniSetting struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Default string `json:"default"`
	Source  string `json:"source"`
}

// Database é um database de usuário.
type Database struct {
	Name   string  `json:"name"`
	SizeMB float64 `json:"sizeMb"`
}

// DB é o banco ativo: conexão e databases. Databases vem vazio com o banco
// fora do ar (DatabasesError diz por quê). Client é o nome do cliente
// ("mysql" ou "mariadb"); Command, a linha que abre esse cliente com o
// caminho completo (vazio sem banco instalado).
type DB struct {
	Engine         string     `json:"engine"`
	Client         string     `json:"client"`
	Command        string     `json:"command"`
	Host           string     `json:"host"`
	Port           int        `json:"port"`
	User           string     `json:"user"`
	Password       string     `json:"password"`
	State          string     `json:"state"`
	Databases      []Database `json:"databases"`
	DatabasesError string     `json:"databasesError,omitempty"`
}

// Warning é um aviso da stack.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Project string `json:"project,omitempty"`
}
