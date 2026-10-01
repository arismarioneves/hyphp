package main

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"text/tabwriter"

	"hyphp/internal/cli"
	"hyphp/internal/i18n"
	"hyphp/internal/version"
)

func table(w io.Writer, header []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()
}

// orDash marca célula vazia. Decoração da saída humana é só ASCII: com a
// saída redirecionada, o PowerShell 5 decodifica no code page OEM e "—", "·"
// viram lixo que ainda desalinha as tabelas.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func printHelp(w io.Writer, name string) {
	l := i18n.Current()
	if name == "" {
		fmt.Fprintln(w, i18n.T("cli.help.header", version.Current))
		fmt.Fprintln(w)
		fmt.Fprintln(w, i18n.T("cli.help.usage"))
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	found := false
	for _, c := range cli.Commands {
		if name != "" && c.Name != name {
			continue
		}
		found = true
		fmt.Fprintf(tw, "  %s\t%s\n", c.Usage(l), c.Description(l))
	}
	tw.Flush()
	if !found {
		fmt.Fprintln(w, i18n.T("cli.err.unknownCommand", name))
	}
	if name == "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, i18n.T("cli.help.options"))
		fmt.Fprintln(w, "  --json   "+i18n.T("cli.help.json"))
		fmt.Fprintln(w)
		fmt.Fprintln(w, i18n.T("cli.help.exit"))
		fmt.Fprintln(w, i18n.T("cli.help.more"))
	}
}

func servicesTable(w io.Writer, list []cli.Service) {
	if len(list) == 0 {
		fmt.Fprintln(w, i18n.T("cli.none.services"))
		return
	}
	rows := make([][]string, 0, len(list))
	for _, s := range list {
		pid, port := "-", "-"
		if s.PID > 0 {
			pid = fmt.Sprint(s.PID)
		}
		if s.Port > 0 {
			port = fmt.Sprint(s.Port)
		}
		state := s.State
		if s.LastError != "" && s.State != "ready" {
			state += " (" + s.LastError + ")"
		}
		rows = append(rows, []string{s.ID, s.Name, state, pid, port})
	}
	table(w, []string{i18n.T("cli.col.id"), i18n.T("cli.col.name"), i18n.T("cli.col.state"), "PID", i18n.T("cli.col.port")}, rows)
}

func printResult(w io.Writer, cmd string, out any) {
	switch v := out.(type) {
	case *cli.Status:
		printStatus(w, *v)
	case *[]cli.Service:
		servicesTable(w, *v)
	case *[]cli.Project:
		if len(*v) == 0 {
			fmt.Fprintln(w, i18n.T("cli.none.projects"))
			return
		}
		rows := make([][]string, 0, len(*v))
		for _, p := range *v {
			rows = append(rows, []string{p.ID, p.Domain, orDash(p.PHP), orDash(p.Database), p.Root})
		}
		table(w, []string{i18n.T("cli.col.project"), i18n.T("cli.col.domain"), "PHP", i18n.T("cli.col.database"), i18n.T("cli.col.folder")}, rows)
	case *[]cli.PHP:
		if len(*v) == 0 {
			fmt.Fprintln(w, i18n.T("cli.none.php"))
			return
		}
		rows := make([][]string, 0, len(*v))
		for _, p := range *v {
			ver := p.Version
			if p.Default {
				ver += " (" + i18n.T("cli.php.default") + ")"
			}
			rows = append(rows, []string{p.Major, ver, p.Dir})
		}
		table(w, []string{i18n.T("cli.col.series"), i18n.T("cli.col.version"), i18n.T("cli.col.folder")}, rows)
	case *[]cli.IniSetting:
		rows := make([][]string, 0, len(*v))
		for _, s := range *v {
			src := i18n.T("cli.ini.source." + s.Source)
			if s.Source == "user" {
				src += " (" + i18n.T("cli.ini.defaultIs", orDash(s.Default)) + ")"
			}
			rows = append(rows, []string{s.Name, orDash(s.Value), src})
		}
		table(w, []string{i18n.T("cli.col.directive"), i18n.T("cli.col.value"), i18n.T("cli.col.source")}, rows)
	case *cli.DB:
		printDB(w, *v)
	case *[]cli.Warning:
		if len(*v) == 0 {
			fmt.Fprintln(w, i18n.T("cli.none.warnings"))
			return
		}
		for _, x := range *v {
			fmt.Fprintf(w, "- %s  (%s)\n", x.Message, x.Code)
		}
	default:
		writeJSON(w, out)
	}
}

func printStatus(w io.Writer, s cli.Status) {
	fmt.Fprintf(w, "HyPHP %s - %s\n", s.Version, s.Root)
	svc := i18n.T("cli.status.ready", s.Ready, s.Total)
	if s.Failed > 0 {
		svc += ", " + i18n.T("cli.status.failed", s.Failed)
	}
	rows := [][]string{
		{i18n.T("cli.status.web"), orDash(s.WebServer)},
		{i18n.T("cli.status.db"), orDash(s.DBEngine)},
		{i18n.T("cli.status.php"), orDash(s.DefaultPHP)},
		{i18n.T("cli.status.services"), svc},
		{i18n.T("cli.status.warnings"), fmt.Sprint(s.Warnings)},
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		fmt.Fprintln(tw, r[0]+"\t"+r[1])
	}
	tw.Flush()
}

func printDB(w io.Writer, d cli.DB) {
	pass := d.Password
	if pass == "" {
		pass = i18n.T("cli.db.emptyPassword")
	}
	conn := orDash(d.Command)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range [][]string{
		{i18n.T("cli.db.engine"), orDash(d.Engine) + " (" + d.State + ")"},
		{"Host", net.JoinHostPort(d.Host, strconv.Itoa(d.Port))},
		{i18n.T("cli.db.user"), d.User},
		{i18n.T("cli.db.password"), pass},
		{i18n.T("cli.db.connection"), conn},
	} {
		fmt.Fprintln(tw, r[0]+"\t"+r[1])
	}
	tw.Flush()
	fmt.Fprintln(w)
	if d.DatabasesError != "" {
		fmt.Fprintln(w, d.DatabasesError)
		return
	}
	if len(d.Databases) == 0 {
		fmt.Fprintln(w, i18n.T("cli.none.databases"))
		return
	}
	rows := make([][]string, 0, len(d.Databases))
	for _, x := range d.Databases {
		rows = append(rows, []string{x.Name, fmt.Sprintf("%.1f MB", x.SizeMB)})
	}
	table(w, []string{i18n.T("cli.col.database"), i18n.T("cli.col.size")}, rows)
}
