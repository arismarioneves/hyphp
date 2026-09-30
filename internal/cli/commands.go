package cli

import "hyphp/internal/i18n"

// Command é uma linha da ajuda: o uso ("hyphp start [serviço|all]") e o que
// faz. Name é a primeira palavra; `hyphp help <Name>` mostra todas as linhas
// dele. A mesma tabela alimenta a ajuda da CLI e a lista da aba CLI do app,
// então as duas nunca divergem.
type Command struct {
	Name string
	key  string // cli.use.<key> e cli.desc.<key> em internal/i18n
}

// Commands na ordem da ajuda.
var Commands = []Command{
	{"status", "status"},
	{"services", "services"},
	{"start", "start"},
	{"stop", "stop"},
	{"restart", "restart"},
	{"logs", "logs"},
	{"projects", "projects"},
	{"php", "php"},
	{"php", "php.default"},
	{"php", "php.use"},
	{"ini", "ini"},
	{"ini", "ini.set"},
	{"ini", "ini.reset"},
	{"db", "db"},
	{"db", "db.create"},
	{"db", "db.drop"},
	{"db", "db.engine"},
	{"web", "web"},
	{"warnings", "warnings"},
	{"app", "app"},
	{"version", "version"},
	{"help", "help"},
}

// Usage é a linha de uso no idioma dado.
func (c Command) Usage(l i18n.Lang) string { return i18n.TIn(l, "cli.use."+c.key) }

// Description diz o que o comando faz, no idioma dado.
func (c Command) Description(l i18n.Lang) string { return i18n.TIn(l, "cli.desc."+c.key) }
