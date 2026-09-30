package services

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hyphp/internal/cli"
	"hyphp/internal/supervisor"
)

func cliDeTeste(t *testing.T) *CLIService {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sup, err := supervisor.New(logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sup.Close() })
	return NewCLIService(CLIDeps{Sup: sup, Logger: logger, Services: NewServicesService(ServicesDeps{Sup: sup, Logger: logger})})
}

func chamar(c *CLIService, cmd, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, cli.CallPath+cmd, strings.NewReader(body))
	r.SetPathValue("cmd", cmd)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	c.handleCall(w, r)
	return w
}

// Erro de argumento é 400 (culpa de quem chamou), comando inexistente é 404,
// e os dois aparecem na atividade da aba com o que foi digitado.
func TestCLIErrosDeChamada(t *testing.T) {
	c := cliDeTeste(t)
	if w := chamar(c, "sumir", `{}`, nil); w.Code != http.StatusNotFound {
		t.Errorf("comando inexistente: %d %s", w.Code, w.Body)
	}
	if w := chamar(c, cli.CmdRestart, `{}`, map[string]string{cli.HeaderArgv: "restart", cli.HeaderCaller: "bash.exe ← claude.exe"}); w.Code != http.StatusBadRequest {
		t.Errorf("restart sem id: %d %s", w.Code, w.Body)
	}
	if w := chamar(c, cli.CmdStart, `{"id": 5}`, nil); w.Code != http.StatusBadRequest {
		t.Errorf("argumento com tipo errado: %d %s", w.Code, w.Body)
	}
	act := c.Activity()
	if len(act) != 3 {
		t.Fatalf("atividade = %d entradas", len(act))
	}
	// A mais nova primeiro.
	if act[1].Command != "restart" || act[1].Caller != "bash.exe ← claude.exe" || act[1].OK || act[1].Error == "" {
		t.Errorf("registro do restart = %+v", act[1])
	}
}

// O resultado de sucesso é JSON do tipo publicado em internal/cli.
func TestCLIServicosEmJSON(t *testing.T) {
	c := cliDeTeste(t)
	w := chamar(c, cli.CmdServices, `{}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var list []cli.Service
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("resposta não é []cli.Service: %v (%s)", err, w.Body)
	}
	if !c.Activity()[0].OK {
		t.Error("chamada certa registrada como erro")
	}
}

// A atividade guarda só as últimas cliActivityMax.
func TestCLIAtividadeLimitada(t *testing.T) {
	c := cliDeTeste(t)
	for range cliActivityMax + 5 {
		chamar(c, cli.CmdServices, `{}`, nil)
	}
	if n := len(c.Activity()); n != cliActivityMax {
		t.Errorf("atividade = %d, want %d", n, cliActivityMax)
	}
}

func TestCLILogsDeServicoInexistente(t *testing.T) {
	c := cliDeTeste(t)
	r := httptest.NewRequest(http.MethodPost, cli.LogsPath, strings.NewReader(`{"id":"nada"}`))
	w := httptest.NewRecorder()
	c.handleLogs(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}
