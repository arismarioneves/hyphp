// Command hyphp é a CLI do HyPHP: cada comando é uma chamada ao app aberto,
// pelo pipe do usuário (internal/cli). O app executa com os mesmos serviços
// da UI; a CLI só traduz argumentos e formata a resposta.
//
//	hyphp status
//	hyphp start all --json
//	hyphp logs mysql -f
//
// Códigos de saída: 0 ok, 1 erro do app, 2 uso errado, 3 app fechado.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"hyphp/internal/cli"
	"hyphp/internal/i18n"
	"hyphp/internal/version"
)

const (
	exitOK    = 0
	exitErr   = 1
	exitUsage = 2
	exitNoApp = 3
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// opts são as opções que valem em qualquer posição da linha de comando.
type opts struct {
	json   bool
	follow bool
	reset  bool
	yes    bool
	help   bool
	lines  int
}

// usageError é argumento errado: sai com 2 e a linha de uso do comando.
type usageError struct {
	msg string
	cmd string // Name da tabela de comandos, para mostrar o uso
}

func (e usageError) Error() string { return e.msg }

// splitArgs separa opções de posicionais.
func splitArgs(args []string) ([]string, opts, error) {
	var o opts
	var pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			o.json = true
		case a == "-f" || a == "--follow":
			o.follow = true
		case a == "--reset":
			o.reset = true
		case a == "--yes" || a == "-y":
			o.yes = true
		case a == "-h" || a == "--help":
			o.help = true
		case a == "-n" || a == "--lines":
			if i+1 >= len(args) {
				return nil, o, usageError{msg: i18n.T("cli.err.flagValue", a), cmd: "logs"}
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				return nil, o, usageError{msg: i18n.T("cli.err.flagValue", a), cmd: "logs"}
			}
			o.lines = n
			i++
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return nil, o, usageError{msg: i18n.T("cli.err.unknownFlag", a)}
		default:
			pos = append(pos, a)
		}
	}
	return pos, o, nil
}

// call é o que um comando pede ao app.
type call struct {
	cmd  string // cli.Cmd*; "" em comandos locais
	args any
	out  any
	logs *cli.LogsArgs
}

// parse traduz os posicionais num pedido ao app. Comandos locais (help,
// version, app) voltam com local preenchido.
func parse(pos []string, o opts) (c call, local string, err error) {
	if len(pos) == 0 || o.help && len(pos) == 1 {
		name := ""
		if len(pos) == 1 {
			name = pos[0]
		}
		return call{}, "help " + name, nil
	}
	name, rest := pos[0], pos[1:]
	need := func(n int) error {
		if len(rest) != n {
			return usageError{msg: i18n.T("cli.err.args", name), cmd: name}
		}
		return nil
	}
	switch name {
	case "help":
		if len(rest) > 1 {
			return call{}, "", need(1)
		}
		return call{}, "help " + strings.Join(rest, ""), nil
	case "version":
		return call{cmd: cli.CmdStatus, out: &cli.Status{}}, "version", need(0)
	case "app":
		return call{}, "app", need(0)
	case "status":
		return call{cmd: cli.CmdStatus, out: &cli.Status{}}, "", need(0)
	case "services":
		return call{cmd: cli.CmdServices, out: &[]cli.Service{}}, "", need(0)
	case "start", "stop":
		if len(rest) > 1 {
			return call{}, "", need(1)
		}
		cmd := cli.CmdStart
		if name == "stop" {
			cmd = cli.CmdStop
		}
		return call{cmd: cmd, args: cli.ServiceArgs{ID: strings.Join(rest, "")}, out: &[]cli.Service{}}, "", nil
	case "restart":
		if err := need(1); err != nil {
			return call{}, "", err
		}
		return call{cmd: cli.CmdRestart, args: cli.ServiceArgs{ID: rest[0]}, out: &[]cli.Service{}}, "", nil
	case "logs":
		if err := need(1); err != nil {
			return call{}, "", err
		}
		return call{logs: &cli.LogsArgs{ID: rest[0], Lines: o.lines, Follow: o.follow}}, "", nil
	case "projects":
		return call{cmd: cli.CmdProjects, out: &[]cli.Project{}}, "", need(0)
	case "php":
		switch {
		case len(rest) == 0:
			return call{cmd: cli.CmdPHP, out: &[]cli.PHP{}}, "", nil
		case rest[0] == "default" && len(rest) == 2:
			return call{cmd: cli.CmdPHPDefault, args: cli.PHPDefaultArgs{Major: rest[1]}, out: &[]cli.PHP{}}, "", nil
		case rest[0] == "use" && len(rest) == 3:
			return call{cmd: cli.CmdPHPUse, args: cli.PHPUseArgs{Project: rest[1], Major: rest[2]}, out: &[]cli.Project{}}, "", nil
		}
		return call{}, "", usageError{msg: i18n.T("cli.err.args", name), cmd: name}
	case "ini":
		switch {
		case len(rest) == 1:
			return call{cmd: cli.CmdIni, args: cli.IniArgs{Major: rest[0]}, out: &[]cli.IniSetting{}}, "", nil
		case len(rest) == 2 && o.reset:
			return call{cmd: cli.CmdIniReset, args: cli.IniArgs{Major: rest[0], Name: rest[1]}, out: &[]cli.IniSetting{}}, "", nil
		case len(rest) == 3 && !o.reset:
			return call{cmd: cli.CmdIniSet, args: cli.IniArgs{Major: rest[0], Name: rest[1], Value: rest[2]}, out: &[]cli.IniSetting{}}, "", nil
		}
		return call{}, "", usageError{msg: i18n.T("cli.err.args", name), cmd: name}
	case "db":
		switch {
		case len(rest) == 0:
			return call{cmd: cli.CmdDB, out: &cli.DB{}}, "", nil
		case rest[0] == "create" && len(rest) == 2:
			return call{cmd: cli.CmdDBCreate, args: cli.NameArgs{Name: rest[1]}, out: &cli.DB{}}, "", nil
		case rest[0] == "drop" && len(rest) == 2:
			// Apagar é definitivo: sem --yes, nem chega no app. Um agente
			// que erra a sintaxe não apaga um banco por acidente.
			if !o.yes {
				return call{}, "", usageError{msg: i18n.T("cli.err.dropNeedsYes", rest[1]), cmd: name}
			}
			return call{cmd: cli.CmdDBDrop, args: cli.NameArgs{Name: rest[1]}, out: &cli.DB{}}, "", nil
		case rest[0] == "engine" && len(rest) == 2:
			return call{cmd: cli.CmdDBEngine, args: cli.NameArgs{Name: rest[1]}, out: &cli.Status{}}, "", nil
		}
		return call{}, "", usageError{msg: i18n.T("cli.err.args", name), cmd: name}
	case "web":
		if err := need(1); err != nil {
			return call{}, "", err
		}
		return call{cmd: cli.CmdWeb, args: cli.NameArgs{Name: rest[0]}, out: &cli.Status{}}, "", nil
	case "warnings":
		return call{cmd: cli.CmdWarnings, out: &[]cli.Warning{}}, "", need(0)
	}
	return call{}, "", usageError{msg: i18n.T("cli.err.unknownCommand", name)}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	// Até o app responder, o texto sai no idioma do Windows; depois, no do app.
	i18n.SetCurrent(i18n.Resolve(""))
	pos, o, err := splitArgs(args)
	var c call
	var local string
	if err == nil {
		c, local, err = parse(pos, o)
	}
	if err != nil {
		return fail(stdout, stderr, o, err)
	}
	addr, err := cli.Address()
	if err != nil {
		return fail(stdout, stderr, o, err)
	}
	client := cli.NewClient(addr, callerName())
	client.Argv = strings.Join(quoteArgs(args), " ")

	switch {
	case strings.HasPrefix(local, "help"):
		printHelp(stdout, strings.TrimSpace(strings.TrimPrefix(local, "help")))
		return exitOK
	case local == "app":
		return runApp(ctx, client, stdout, stderr, o)
	case c.logs != nil:
		err = client.Logs(ctx, *c.logs, func(l string) {
			if o.json {
				b, _ := json.Marshal(struct {
					Line string `json:"line"`
				}{l})
				fmt.Fprintln(stdout, string(b))
				return
			}
			fmt.Fprintln(stdout, l)
		})
		return done(stdout, stderr, o, client, err)
	}

	if local == "version" {
		return runVersion(ctx, client, stdout, stderr, o, c)
	}

	if err := client.Call(ctx, c.cmd, c.args, c.out); err != nil {
		return done(stdout, stderr, o, client, err)
	}
	adoptLang(client)
	if o.json {
		writeJSON(stdout, c.out)
		return exitOK
	}
	printResult(stdout, c.cmd, c.out)
	return exitOK
}

// runVersion imprime a versão da CLI, que está no próprio binário, e a do app
// quando ele responde. Com o app fechado só a parte do app some: scripts que
// conferem a versão instalada não podem depender do app aberto.
func runVersion(ctx context.Context, client *cli.Client, stdout, stderr io.Writer, o opts, c call) int {
	out := struct {
		CLI string `json:"cli"`
		App string `json:"app"`
	}{CLI: version.Current}
	switch err := client.Call(ctx, c.cmd, c.args, c.out); {
	case err == nil:
		adoptLang(client)
		out.App = c.out.(*cli.Status).Version
	case !errors.Is(err, cli.ErrAppNotRunning):
		return done(stdout, stderr, o, client, err)
	}
	switch {
	case o.json:
		writeJSON(stdout, out)
	case out.App == "":
		fmt.Fprintln(stdout, i18n.T("cli.versionOnly", out.CLI))
	default:
		fmt.Fprintln(stdout, i18n.T("cli.version", out.CLI, out.App))
	}
	return exitOK
}

// runApp mostra a janela do app aberto ou abre o app e espera o pipe
// responder, para o próximo comando (de uma pessoa ou de um agente) já achar
// o app de pé.
func runApp(ctx context.Context, client *cli.Client, stdout, stderr io.Writer, o opts) int {
	err := client.Call(ctx, cli.CmdShowWindow, nil, nil)
	if err == nil {
		adoptLang(client)
		say(stdout, o, i18n.T("cli.app.shown"))
		return exitOK
	}
	if !errors.Is(err, cli.ErrAppNotRunning) {
		return done(stdout, stderr, o, client, err)
	}
	if err := startGUI(); err != nil {
		return fail(stdout, stderr, o, err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if err := client.Call(ctx, cli.CmdStatus, nil, nil); err == nil {
			adoptLang(client)
			say(stdout, o, i18n.T("cli.app.started"))
			return exitOK
		}
		select {
		case <-ctx.Done():
			return exitErr
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fail(stdout, stderr, o, errors.New(i18n.T("cli.app.timeout")))
}

// adoptLang passa o texto da CLI para o idioma do app.
func adoptLang(c *cli.Client) {
	if c.Lang != "" && i18n.Valid(c.Lang) {
		i18n.SetCurrent(i18n.Resolve(c.Lang))
	}
}

// done trata o fim de uma chamada ao app.
func done(stdout, stderr io.Writer, o opts, c *cli.Client, err error) int {
	if err == nil {
		return exitOK
	}
	adoptLang(c)
	return fail(stdout, stderr, o, err)
}

// fail escreve o erro e devolve o código de saída. Com --json o erro também
// sai em JSON, no stdout, para quem lê a saída não ter dois formatos.
func fail(stdout, stderr io.Writer, o opts, err error) int {
	code := exitErr
	msg := err.Error()
	var ue usageError
	switch {
	case errors.Is(err, cli.ErrAppNotRunning):
		code, msg = exitNoApp, i18n.T("cli.err.appNotRunning")
	case errors.As(err, &ue):
		code = exitUsage
	}
	if o.json {
		writeJSON(stdout, struct {
			Error string `json:"error"`
			Code  int    `json:"code"`
		}{msg, code})
		return code
	}
	fmt.Fprintln(stderr, "hyphp: "+msg)
	if ue.cmd != "" {
		for _, c := range cli.Commands {
			if c.Name == ue.cmd {
				fmt.Fprintln(stderr, "  "+c.Usage(i18n.Current()))
			}
		}
	} else if code == exitUsage {
		fmt.Fprintln(stderr, i18n.T("cli.help.more"))
	}
	return code
}

func say(stdout io.Writer, o opts, msg string) {
	if o.json {
		writeJSON(stdout, struct {
			Message string `json:"message"`
		}{msg})
		return
	}
	fmt.Fprintln(stdout, msg)
}

// writeJSON escreve JSON só em ASCII: acentos saem como \u00e9. Quem lê pela
// saída capturada (o PowerShell 5 decodifica em cp850, um agente de IA num
// shell qualquer) recebe o mesmo texto em qualquer página de código.
func writeJSON(w io.Writer, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintln(w, `{"error":"json"}`)
		return
	}
	var out strings.Builder
	for _, r := range string(b) {
		switch {
		case r < 0x80:
			out.WriteRune(r)
		case r > 0xffff:
			// Fora do plano básico: par substituto, como o JSON pede.
			r1, r2 := utf16.EncodeRune(r)
			fmt.Fprintf(&out, `\u%04x\u%04x`, r1, r2)
		default:
			fmt.Fprintf(&out, `\u%04x`, r)
		}
	}
	fmt.Fprintln(w, out.String())
}

// quoteArgs devolve os argumentos como seriam digitados, para a atividade da
// aba CLI mostrar a linha de comando.
func quoteArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\"") {
			a = strconv.Quote(a)
		}
		out[i] = a
	}
	return out
}
