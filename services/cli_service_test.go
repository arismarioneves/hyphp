package services

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// servirDeTeste sobe o servidor da CLI num listener TCP de verdade (o pipe
// do go-winio também respeita prazos) com o prazo de leitura encurtado.
func servirDeTeste(t *testing.T, c *CLIService) string {
	t.Helper()
	c.readTimeout = 150 * time.Millisecond
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c.serve(l, l.Addr().String())
	t.Cleanup(func() { _ = c.ServiceShutdown() })
	return l.Addr().String()
}

// Um cliente que manda os cabeçalhos e trava no corpo não pode prender o
// handler no Decode para sempre.
func TestCLICorpoTravadoExpira(t *testing.T) {
	c := cliDeTeste(t)
	addr := servirDeTeste(t, c)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	req := "POST " + cli.CallPath + cli.CmdServices + " HTTP/1.1\r\nHost: hyphp\r\nContent-Length: 100\r\n\r\n{"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if ne, ok := errors.AsType[net.Error](err); ok && ne.Timeout() {
		t.Fatal("servidor continua esperando o corpo depois do prazo")
	}
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// O prazo de leitura vale só até o corpo: `logs -f` continua recebendo
// linhas bem depois de ele ter vencido.
func TestCLILogsSeguemDepoisDoPrazoDeLeitura(t *testing.T) {
	c := cliDeTeste(t)
	if err := c.d.Sup.Add(supervisor.Spec{ID: "web", Exe: "cmd.exe", Probe: supervisor.AliveProbe{}}); err != nil {
		t.Fatal(err)
	}
	ring, _ := c.d.Sup.Logs("web")
	addr := servirDeTeste(t, c)

	resp, err := http.Post("http://"+addr+cli.LogsPath, "application/json", strings.NewReader(`{"id":"web","follow":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	lines := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()

	time.Sleep(3 * c.readTimeout)
	_, _ = io.WriteString(ring, "depois do prazo\n")
	select {
	case l, ok := <-lines:
		if !ok {
			t.Fatal("stream fechou depois do prazo do corpo")
		}
		if l != "depois do prazo" {
			t.Errorf("linha = %q", l)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("linha não chegou")
	}
}
