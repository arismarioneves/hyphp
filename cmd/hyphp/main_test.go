package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"hyphp/internal/cli"
	"hyphp/internal/version"
)

func parseLine(t *testing.T, args ...string) (call, string, error) {
	t.Helper()
	pos, o, err := splitArgs(args)
	if err != nil {
		return call{}, "", err
	}
	return parse(pos, o)
}

func TestParseViraOComandoCerto(t *testing.T) {
	casos := []struct {
		args []string
		cmd  string
		want any
	}{
		{[]string{"start"}, cli.CmdStart, cli.ServiceArgs{}},
		{[]string{"start", "mysql"}, cli.CmdStart, cli.ServiceArgs{ID: "mysql"}},
		{[]string{"stop", "all"}, cli.CmdStop, cli.ServiceArgs{ID: "all"}},
		{[]string{"php", "use", "blog", "8.3"}, cli.CmdPHPUse, cli.PHPUseArgs{Project: "blog", Major: "8.3"}},
		{[]string{"ini", "7.4", "max_input_vars", "5000"}, cli.CmdIniSet, cli.IniArgs{Major: "7.4", Name: "max_input_vars", Value: "5000"}},
		{[]string{"ini", "7.4", "max_input_vars", "--reset"}, cli.CmdIniReset, cli.IniArgs{Major: "7.4", Name: "max_input_vars"}},
		{[]string{"--json", "db", "create", "acme"}, cli.CmdDBCreate, cli.NameArgs{Name: "acme"}},
		{[]string{"db", "drop", "acme", "--yes"}, cli.CmdDBDrop, cli.NameArgs{Name: "acme"}},
	}
	for _, c := range casos {
		got, _, err := parseLine(t, c.args...)
		if err != nil {
			t.Errorf("%v: %v", c.args, err)
			continue
		}
		if got.cmd != c.cmd || !reflect.DeepEqual(got.args, c.want) {
			t.Errorf("%v → %s %+v, want %s %+v", c.args, got.cmd, got.args, c.cmd, c.want)
		}
	}
}

// Apagar database sem --yes não pode nem chegar ao app, e valor faltando no
// ini não pode virar "definir vazio".
func TestParseRecusaSemConfirmacaoOuIncompleto(t *testing.T) {
	for _, args := range [][]string{
		{"db", "drop", "acme"},
		{"ini", "7.4", "max_input_vars"},
		{"restart"},
		{"logs", "mysql", "-n", "zero"},
		{"sumir"},
		{"status", "--forca"},
	} {
		_, _, err := parseLine(t, args...)
		var ue usageError
		if !errors.As(err, &ue) {
			t.Errorf("%v: err = %v, want usageError", args, err)
		}
	}
}

func TestLogsLeOpcoes(t *testing.T) {
	got, _, err := parseLine(t, "logs", "php:8.3:0", "-n", "200", "-f")
	if err != nil {
		t.Fatal(err)
	}
	if got.logs == nil || *got.logs != (cli.LogsArgs{ID: "php:8.3:0", Lines: 200, Follow: true}) {
		t.Errorf("logs = %+v", got.logs)
	}
}

// Com o app fechado a saída é 3, e com --json o erro também sai em JSON:
// um agente de IA decide pelo código e pelo campo, sem ler texto.
func TestAppFechadoSai3(t *testing.T) {
	t.Setenv(cli.EnvPipe, fmt.Sprintf(`\\.\pipe\hyphp-teste-fechado-%d-%d`, os.Getpid(), time.Now().UnixNano()))
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"status", "--json"}, &out, &errOut); code != exitNoApp {
		t.Fatalf("código = %d, want %d", code, exitNoApp)
	}
	var body struct {
		Error string `json:"error"`
		Code  int    `json:"code"`
	}
	if err := json.Unmarshal(out.Bytes(), &body); err != nil || body.Code != exitNoApp || body.Error == "" {
		t.Errorf("saída = %q (%v)", out.String(), err)
	}
}

// A versão da CLI está no binário: com o app fechado `version` sai 0 e só
// deixa a parte do app vazia.
func TestVersionComAppFechado(t *testing.T) {
	t.Setenv(cli.EnvPipe, fmt.Sprintf(`\\.\pipe\hyphp-teste-versao-%d-%d`, os.Getpid(), time.Now().UnixNano()))
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"version", "--json"}, &out, &errOut); code != exitOK {
		t.Fatalf("código = %d, want %d (%s)", code, exitOK, out.String())
	}
	var body struct {
		CLI string `json:"cli"`
		App string `json:"app"`
	}
	if err := json.Unmarshal(out.Bytes(), &body); err != nil || body.CLI != version.Current || body.App != "" {
		t.Errorf("saída = %q (%v)", out.String(), err)
	}
}

// A saída --json é lida por programas em shells com página de código
// qualquer: tem de ser ASCII puro e decodificar para o texto original.
func TestJSONSoASCII(t *testing.T) {
	var buf bytes.Buffer
	in := map[string]string{"msg": "apagar é definitivo · 🙂"}
	writeJSON(&buf, in)
	for _, b := range buf.Bytes() {
		if b >= 0x80 {
			t.Fatalf("byte não ASCII na saída: %q", buf.String())
		}
	}
	var out map[string]string
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil || out["msg"] != in["msg"] {
		t.Errorf("volta = %q (%v)", out["msg"], err)
	}
}
