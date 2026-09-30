package services

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"hyphp/internal/cli"
	"hyphp/internal/i18n"
	"hyphp/internal/runtime"
	"hyphp/internal/stack"
	"hyphp/internal/supervisor"
)

type cliHandler func(ctx context.Context, raw json.RawMessage) (any, error)

// decodeArgs lê os argumentos de um comando. Erro aqui é culpa de quem chamou
// (400), não do app.
func decodeArgs[T any](cmd string, raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 || string(raw) == "null" {
		return v, nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, argError{i18n.Errorf("err.cli.badArgs", cmd, err)}
	}
	return v, nil
}

func missing(cmd, arg string) error {
	return argError{i18n.Errorf("err.cli.missing", cmd, arg)}
}

func (c *CLIService) handlers() map[string]cliHandler {
	return map[string]cliHandler{
		cli.CmdStatus:     c.cliStatus,
		cli.CmdServices:   func(context.Context, json.RawMessage) (any, error) { return cliServices(c.d.Services.List()), nil },
		cli.CmdStart:      c.cliStart,
		cli.CmdStop:       c.cliStop,
		cli.CmdRestart:    c.cliRestart,
		cli.CmdProjects:   c.cliProjects,
		cli.CmdPHP:        func(context.Context, json.RawMessage) (any, error) { return c.cliPHPList(), nil },
		cli.CmdPHPDefault: c.cliPHPDefault,
		cli.CmdPHPUse:     c.cliPHPUse,
		cli.CmdIni:        c.cliIni,
		cli.CmdIniSet:     c.cliIniSet,
		cli.CmdIniReset:   c.cliIniReset,
		cli.CmdDB:         func(context.Context, json.RawMessage) (any, error) { return c.cliDB(), nil },
		cli.CmdDBCreate:   c.cliDBCreate,
		cli.CmdDBDrop:     c.cliDBDrop,
		cli.CmdDBEngine:   c.cliDBEngine,
		cli.CmdWeb:        c.cliWeb,
		cli.CmdWarnings:   func(context.Context, json.RawMessage) (any, error) { return c.cliWarnings(), nil },
		cli.CmdShowWindow: c.cliShowWindow,
	}
}

func (c *CLIService) cliStatus(context.Context, json.RawMessage) (any, error) {
	st := c.d.Settings.Get()
	out := cli.Status{
		Version:    c.d.App.Version(),
		Root:       c.d.App.RuntimeRoot(),
		Language:   string(i18n.Current()),
		WebServer:  string(st.WebServer),
		DefaultPHP: c.defaultMajor(),
		Warnings:   len(c.d.Settings.Warnings()),
	}
	if inst, ok := stack.DBRuntime(c.d.Runtimes.Installed(), st); ok {
		out.DBEngine = string(inst.Kind)
	}
	for _, s := range c.d.Services.List() {
		out.Total++
		switch s.State {
		case supervisor.Ready:
			out.Ready++
		case supervisor.Failed, supervisor.Degraded:
			out.Failed++
		}
	}
	return out, nil
}

// defaultMajor é a regra do Reconcile (stack/desired.go): state.DefaultPHP
// manda e, vazio, vale a maior série instalada.
func (c *CLIService) defaultMajor() string {
	if m := c.d.Settings.Get().DefaultPHP; m != "" {
		return m
	}
	return stack.HighestPHPMajor(runtime.ByKind(c.d.Runtimes.Installed(), runtime.PHP))
}

func cliServices(list []supervisor.Status) []cli.Service {
	out := make([]cli.Service, 0, len(list))
	for _, s := range list {
		out = append(out, cli.Service{
			ID: s.ID, Name: s.Name, Group: s.Group, State: string(s.State),
			PID: s.PID, Port: s.Port, Restarts: s.Restarts, LastError: s.LastError,
		})
	}
	return out
}

// cliSettleTimeout limita a espera de start/restart: o MySQL na primeira
// subida é o mais lento (o probe dele espera 60 s).
const cliSettleTimeout = 90 * time.Second

// settle espera os serviços dados saírem de starting/stopping, para a CLI
// responder "ready" ou "failed" em vez de "starting". ids nil = todos.
func (c *CLIService) settle(ctx context.Context, ids []string) []cli.Service {
	deadline := time.Now().Add(cliSettleTimeout)
	for {
		var sel []supervisor.Status
		busy := false
		for _, s := range c.d.Services.List() {
			if ids != nil && !slices.Contains(ids, s.ID) {
				continue
			}
			sel = append(sel, s)
			if s.State == supervisor.Starting || s.State == supervisor.Stopping {
				busy = true
			}
		}
		if !busy || time.Now().After(deadline) {
			return cliServices(sel)
		}
		select {
		case <-ctx.Done():
			return cliServices(sel)
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (c *CLIService) cliStart(ctx context.Context, raw json.RawMessage) (any, error) {
	a, err := decodeArgs[cli.ServiceArgs](cli.CmdStart, raw)
	if err != nil {
		return nil, err
	}
	if a.ID == "" || a.ID == cli.AllServices {
		if err := c.d.Services.StartAll(); err != nil {
			return nil, err
		}
		return c.settle(ctx, nil), nil
	}
	if err := c.knownService(a.ID); err != nil {
		return nil, err
	}
	if err := c.d.Services.Start(a.ID); err != nil {
		return nil, err
	}
	return c.settle(ctx, []string{a.ID}), nil
}

func (c *CLIService) cliStop(ctx context.Context, raw json.RawMessage) (any, error) {
	a, err := decodeArgs[cli.ServiceArgs](cli.CmdStop, raw)
	if err != nil {
		return nil, err
	}
	if a.ID == "" || a.ID == cli.AllServices {
		if err := c.d.Services.StopAll(); err != nil {
			return nil, err
		}
		return c.settle(ctx, nil), nil
	}
	if err := c.knownService(a.ID); err != nil {
		return nil, err
	}
	if err := c.d.Services.Stop(a.ID); err != nil {
		return nil, err
	}
	return c.settle(ctx, []string{a.ID}), nil
}

func (c *CLIService) cliRestart(ctx context.Context, raw json.RawMessage) (any, error) {
	a, err := decodeArgs[cli.ServiceArgs](cli.CmdRestart, raw)
	if err != nil {
		return nil, err
	}
	if a.ID == "" {
		return nil, missing(cli.CmdRestart, "id")
	}
	if err := c.knownService(a.ID); err != nil {
		return nil, err
	}
	if err := c.d.Services.Restart(a.ID); err != nil {
		return nil, err
	}
	return c.settle(ctx, []string{a.ID}), nil
}

// knownService recusa id que o supervisor não conhece com a mesma mensagem
// dos logs; sem isso o restart juntaria o erro do stop e o do start.
func (c *CLIService) knownService(id string) error {
	if _, ok := c.d.Sup.Status(id); !ok {
		return argError{i18n.Errorf("err.cli.unknownService", id)}
	}
	return nil
}

func (c *CLIService) cliProjects(context.Context, json.RawMessage) (any, error) {
	list, err := c.d.Projects.List()
	if err != nil {
		return nil, err
	}
	out := make([]cli.Project, 0, len(list))
	for _, p := range list {
		php := p.PHPEffective
		if php == "" {
			php = p.PHP
		}
		out = append(out, cli.Project{
			ID: p.ID, Name: p.Name, Domain: p.Domain, PHP: php, Root: p.Root,
			Docroot: p.DocrootAbs, Database: p.Database, Manifest: p.HasManifest,
		})
	}
	return out, nil
}

func (c *CLIService) cliPHPList() []cli.PHP {
	def := c.defaultMajor()
	var out []cli.PHP
	for _, i := range runtime.ByKind(c.d.Runtimes.Installed(), runtime.PHP) {
		out = append(out, cli.PHP{Version: i.Version, Major: i.Major, Dir: i.Dir, Default: i.Major == def})
	}
	// Da mais nova para a mais antiga, como a aba Runtimes.
	slices.SortFunc(out, func(a, b cli.PHP) int { return runtime.CompareVersions(b.Version, a.Version) })
	if out == nil {
		out = []cli.PHP{}
	}
	return out
}

func (c *CLIService) cliPHPDefault(_ context.Context, raw json.RawMessage) (any, error) {
	a, err := decodeArgs[cli.PHPDefaultArgs](cli.CmdPHPDefault, raw)
	if err != nil {
		return nil, err
	}
	if a.Major == "" {
		return nil, missing(cli.CmdPHPDefault, "major")
	}
	if err := c.d.Runtimes.SetDefaultPHP(a.Major); err != nil {
		return nil, err
	}
	return c.cliPHPList(), nil
}

func (c *CLIService) cliPHPUse(ctx context.Context, raw json.RawMessage) (any, error) {
	a, err := decodeArgs[cli.PHPUseArgs](cli.CmdPHPUse, raw)
	if err != nil {
		return nil, err
	}
	if a.Project == "" || a.Major == "" {
		return nil, missing(cli.CmdPHPUse, "project, major")
	}
	if err := c.d.Projects.SetPHP(a.Project, a.Major); err != nil {
		return nil, err
	}
	return c.cliProjects(ctx, nil)
}

func cliIni(list []IniSetting) []cli.IniSetting {
	out := make([]cli.IniSetting, 0, len(list))
	for _, s := range list {
		out = append(out, cli.IniSetting{Name: s.Name, Value: s.Value, Default: s.DefaultValue, Source: s.Source})
	}
	return out
}

func (c *CLIService) iniList(major string) (any, error) {
	list, err := c.d.Runtimes.IniSettings(major)
	if err != nil {
		return nil, err
	}
	return cliIni(list), nil
}

func (c *CLIService) cliIni(_ context.Context, raw json.RawMessage) (any, error) {
	a, err := decodeArgs[cli.IniArgs](cli.CmdIni, raw)
	if err != nil {
		return nil, err
	}
	if a.Major == "" {
		return nil, missing(cli.CmdIni, "major")
	}
	return c.iniList(a.Major)
}

func (c *CLIService) cliIniSet(_ context.Context, raw json.RawMessage) (any, error) {
	a, err := decodeArgs[cli.IniArgs](cli.CmdIniSet, raw)
	if err != nil {
		return nil, err
	}
	if a.Major == "" || a.Name == "" {
		return nil, missing(cli.CmdIniSet, "major, name, value")
	}
	if err := c.d.Runtimes.SetIniSetting(a.Major, a.Name, a.Value); err != nil {
		return nil, err
	}
	return c.iniList(a.Major)
}

func (c *CLIService) cliIniReset(_ context.Context, raw json.RawMessage) (any, error) {
	a, err := decodeArgs[cli.IniArgs](cli.CmdIniReset, raw)
	if err != nil {
		return nil, err
	}
	if a.Major == "" || a.Name == "" {
		return nil, missing(cli.CmdIniReset, "major, name")
	}
	if err := c.d.Runtimes.ResetIniSetting(a.Major, a.Name); err != nil {
		return nil, err
	}
	return c.iniList(a.Major)
}

func (c *CLIService) cliDB() cli.DB {
	cr := c.d.Database.Credentials()
	out := cli.DB{
		Engine: cr.Engine, Client: cr.Client, Host: cr.Host, Port: cr.Port, User: cr.User, Password: cr.Password,
		State: string(c.d.Database.Status().State), Databases: []cli.Database{},
	}
	dbs, err := c.d.Database.Databases()
	if err != nil {
		out.DatabasesError = err.Error()
		return out
	}
	for _, d := range dbs {
		out.Databases = append(out.Databases, cli.Database{Name: d.Name, SizeMB: d.SizeMB})
	}
	return out
}

func (c *CLIService) cliDBCreate(_ context.Context, raw json.RawMessage) (any, error) {
	a, err := decodeArgs[cli.NameArgs](cli.CmdDBCreate, raw)
	if err != nil {
		return nil, err
	}
	if a.Name == "" {
		return nil, missing(cli.CmdDBCreate, "name")
	}
	if err := c.d.Database.Create(a.Name); err != nil {
		return nil, err
	}
	return c.cliDB(), nil
}

// A confirmação (--yes) fica na CLI: quem chama o pipe direto já decidiu.
func (c *CLIService) cliDBDrop(_ context.Context, raw json.RawMessage) (any, error) {
	a, err := decodeArgs[cli.NameArgs](cli.CmdDBDrop, raw)
	if err != nil {
		return nil, err
	}
	if a.Name == "" {
		return nil, missing(cli.CmdDBDrop, "name")
	}
	if err := c.d.Database.Drop(a.Name); err != nil {
		return nil, err
	}
	return c.cliDB(), nil
}

func (c *CLIService) cliDBEngine(ctx context.Context, raw json.RawMessage) (any, error) {
	a, err := decodeArgs[cli.NameArgs](cli.CmdDBEngine, raw)
	if err != nil {
		return nil, err
	}
	if a.Name == "" {
		return nil, missing(cli.CmdDBEngine, "name")
	}
	if err := c.d.Settings.SwitchDatabase(strings.ToLower(a.Name)); err != nil {
		return nil, err
	}
	return c.cliStatus(ctx, nil)
}

func (c *CLIService) cliWeb(ctx context.Context, raw json.RawMessage) (any, error) {
	a, err := decodeArgs[cli.NameArgs](cli.CmdWeb, raw)
	if err != nil {
		return nil, err
	}
	if a.Name == "" {
		return nil, missing(cli.CmdWeb, "name")
	}
	if err := c.d.Settings.SwitchWebServer(strings.ToLower(a.Name)); err != nil {
		return nil, err
	}
	return c.cliStatus(ctx, nil)
}

func (c *CLIService) cliWarnings() []cli.Warning {
	out := []cli.Warning{}
	for _, w := range c.d.Settings.Warnings() {
		out = append(out, cli.Warning{Code: w.Code, Message: w.Message, Project: w.ProjectID})
	}
	return out
}

func (c *CLIService) cliShowWindow(context.Context, json.RawMessage) (any, error) {
	if c.d.ShowWindow == nil {
		return nil, errors.New("cli: janela indisponível")
	}
	c.d.ShowWindow()
	return struct{}{}, nil
}
