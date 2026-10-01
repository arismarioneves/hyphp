package services

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"hyphp/internal/cli"
	"hyphp/internal/i18n"
	"hyphp/internal/supervisor"
)

// CLIDeps são as dependências injetadas por main.go. Os serviços são os
// mesmos que a UI usa: o que a CLI faz passa pelo mesmo caminho e emite os
// mesmos eventos, então a janela acompanha na hora.
type CLIDeps struct {
	App      *AppService
	Services *ServicesService
	Projects *ProjectsService
	Runtimes *RuntimesService
	Settings *SettingsService
	Database *DatabaseService
	// Sup dá os logs dos serviços (hyphp logs).
	Sup *supervisor.Supervisor
	// ShowWindow traz a janela para a frente (hyphp app).
	ShowWindow func()
	Emit       func(name string, data any)
	Logger     *slog.Logger
	// Addr é o endereço do pipe; "" vale cli.Address().
	Addr string
	// Exe é o hyphp.exe da CLI, ao lado do app em cli\.
	Exe string
}

// CLIInfo é o estado que a aba CLI mostra.
type CLIInfo struct {
	Address   string `json:"address"`
	Listening bool   `json:"listening"`
	Error     string `json:"error"`
	Exe       string `json:"exe"`
	ExeExists bool   `json:"exeExists"`
	OnPath    bool   `json:"onPath"`
}

// CLICommand é uma linha da lista de comandos da aba.
type CLICommand struct {
	Usage       string `json:"usage"`
	Description string `json:"description"`
}

// CLIActivity é uma chamada recebida da CLI.
type CLIActivity struct {
	At         time.Time `json:"at"`
	Command    string    `json:"command"`
	Caller     string    `json:"caller"`
	OK         bool      `json:"ok"`
	Error      string    `json:"error"`
	DurationMs int64     `json:"durationMs"`
}

// cliActivityMax é quanto da atividade fica guardado (em memória).
const cliActivityMax = 100

// cliReadTimeout limita a leitura de cada requisição (cabeçalhos e corpo):
// sem ele, um cliente que manda os cabeçalhos e para no corpo prende o
// handler no Decode para sempre. Não derruba `logs -f` nem chamadas longas:
// lido o corpo, o net/http zera o prazo da conexão ao iniciar a leitura de
// fundo que vigia a desconexão, e é essa leitura que cancelaria o contexto
// se o prazo continuasse armado. Por isso não se usa SetReadDeadline por
// requisição no handler: armado depois dessa leitura já ter começado (corpo
// vazio), ele a faria estourar e cancelar o contexto (golang/go#70834).
const cliReadTimeout = 10 * time.Second

// CLIService escuta a CLI no pipe do usuário e serve a aba CLI.
type CLIService struct {
	d        CLIDeps
	srv      *http.Server
	mu       sync.Mutex
	addr     string
	listenOK bool
	listenEr string
	activity []CLIActivity // mais antiga primeiro
	// readTimeout é cliReadTimeout; o teste encurta.
	readTimeout time.Duration
}

func NewCLIService(d CLIDeps) *CLIService {
	return &CLIService{d: d, readTimeout: cliReadTimeout}
}

// ServiceStartup abre o pipe quando o app sobe. Falhar aqui não derruba o
// app: a aba mostra o erro e o resto continua funcionando.
func (c *CLIService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	addr := c.d.Addr
	if addr == "" {
		a, err := cli.Address()
		if err != nil {
			c.setListen("", false, err)
			return nil
		}
		addr = a
	}
	l, err := cli.Listen(addr)
	if err != nil {
		c.d.Logger.Warn("cli: pipe indisponível", "addr", addr, "err", err)
		c.setListen(addr, false, err)
		return nil
	}
	// Ouvindo antes do Serve: se ele falhar logo, a goroutine grava a falha
	// por último em vez de ser sobrescrita por este true.
	c.setListen(addr, true, nil)
	c.serve(l, addr)
	c.d.Logger.Info("cli: ouvindo", "addr", addr)
	return nil
}

// ServiceShutdown fecha o pipe e derruba as conexões abertas (logs -f).
func (c *CLIService) ServiceShutdown() error {
	c.mu.Lock()
	srv := c.srv
	c.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Close()
}

func (c *CLIService) serve(l net.Listener, addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+cli.CallPath+"{cmd}", c.handleCall)
	mux.HandleFunc("POST "+cli.LogsPath, c.handleLogs)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: c.readTimeout}
	c.mu.Lock()
	c.srv = srv
	c.mu.Unlock()
	go func() {
		if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			c.d.Logger.Warn("cli: servidor parou", "err", err)
			c.setListen(addr, false, err)
		}
	}()
}

func (c *CLIService) setListen(addr string, ok bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addr, c.listenOK, c.listenEr = addr, ok, ""
	if err != nil {
		c.listenEr = err.Error()
	}
}

// Info devolve o estado da CLI para a aba.
func (c *CLIService) Info() CLIInfo {
	c.mu.Lock()
	info := CLIInfo{Address: c.addr, Listening: c.listenOK, Error: c.listenEr, Exe: c.d.Exe}
	c.mu.Unlock()
	if _, err := os.Stat(c.d.Exe); err == nil {
		info.ExeExists = true
	}
	if p, err := readUserPath(); err == nil {
		_, mudou := pathComDir(p, filepath.Dir(c.d.Exe))
		info.OnPath = !mudou
	}
	return info
}

// Commands lista os comandos no idioma atual do app.
func (c *CLIService) Commands() []CLICommand {
	l := i18n.Current()
	out := make([]CLICommand, 0, len(cli.Commands))
	for _, cmd := range cli.Commands {
		out = append(out, CLICommand{Usage: cmd.Usage(l), Description: cmd.Description(l)})
	}
	return out
}

// Activity devolve as chamadas recentes, a mais nova primeiro.
func (c *CLIService) Activity() []CLIActivity {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]CLIActivity, len(c.activity))
	for i, a := range c.activity {
		out[len(c.activity)-1-i] = a
	}
	return out
}

// AddToPath põe a pasta da CLI no PATH do usuário: terminais abertos depois
// disso acham `hyphp`.
func (c *CLIService) AddToPath() error {
	if _, err := os.Stat(c.d.Exe); err != nil {
		return i18n.Errorf("err.cli.exeMissing", c.d.Exe)
	}
	return addToUserPath(filepath.Dir(c.d.Exe))
}

func (c *CLIService) record(a CLIActivity) {
	c.mu.Lock()
	c.activity = append(c.activity, a)
	if len(c.activity) > cliActivityMax {
		c.activity = c.activity[len(c.activity)-cliActivityMax:]
	}
	c.mu.Unlock()
	if c.d.Emit != nil {
		c.d.Emit("cli:activity", a)
	}
}

// activityOf monta o registro de uma chamada: o que foi digitado (ou, sem o
// cabeçalho, o nome do comando) e quem chamou.
func activityOf(r *http.Request, cmd string, start time.Time, err error) CLIActivity {
	a := CLIActivity{
		At:         start,
		Command:    strings.TrimSpace(r.Header.Get(cli.HeaderArgv)),
		Caller:     r.Header.Get(cli.HeaderCaller),
		OK:         err == nil,
		DurationMs: time.Since(start).Milliseconds(),
	}
	if a.Command == "" {
		a.Command = cmd
	}
	if err != nil {
		a.Error = err.Error()
	}
	return a
}

// argError é um erro de argumento: 400 em vez de 500.
type argError struct{ error }

func (c *CLIService) handleCall(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	cmd := r.PathValue("cmd")
	w.Header().Set(cli.HeaderLang, string(i18n.Current()))
	h, ok := c.handlers()[cmd]
	if !ok {
		err := i18n.Errorf("err.cli.unknownCommand", cmd)
		writeCLIError(w, http.StatusNotFound, err)
		c.record(activityOf(r, cmd, start, err))
		return
	}
	var raw json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&raw); err != nil {
		err = i18n.Errorf("err.cli.badArgs", cmd, err)
		writeCLIError(w, http.StatusBadRequest, err)
		c.record(activityOf(r, cmd, start, err))
		return
	}
	out, err := h(r.Context(), raw)
	c.record(activityOf(r, cmd, start, err))
	if err != nil {
		status := http.StatusInternalServerError
		var ae argError
		if errors.As(err, &ae) {
			status = http.StatusBadRequest
		}
		writeCLIError(w, status, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if out == nil {
		out = struct{}{}
	}
	_ = json.NewEncoder(w).Encode(out)
}

func writeCLIError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(cli.ErrorBody{Error: err.Error()})
}

// cliLogLines é o padrão de `hyphp logs` sem -n.
const cliLogLines = 50

func (c *CLIService) handleLogs(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	w.Header().Set(cli.HeaderLang, string(i18n.Current()))
	var args cli.LogsArgs
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&args); err != nil {
		err = i18n.Errorf("err.cli.badArgs", "logs", err)
		writeCLIError(w, http.StatusBadRequest, err)
		c.record(activityOf(r, "logs", start, err))
		return
	}
	ring, ok := c.d.Sup.Logs(args.ID)
	if !ok {
		err := i18n.Errorf("err.cli.unknownService", args.ID)
		writeCLIError(w, http.StatusNotFound, err)
		c.record(activityOf(r, "logs", start, err))
		return
	}
	c.record(activityOf(r, "logs", start, nil))
	n := args.Lines
	if n <= 0 {
		n = cliLogLines
	}
	// Assina antes do retrato, como o stream da UI: o inverso perderia as
	// linhas escritas entre um e outro.
	var lines <-chan string
	if args.Follow {
		ch, cancel := ring.Subscribe()
		defer cancel()
		lines = ch
	}
	backlog := ring.Lines()
	if len(backlog) > n {
		backlog = backlog[len(backlog)-n:]
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	flusher, _ := w.(http.Flusher)
	for _, l := range backlog {
		if _, err := w.Write([]byte(strings.TrimRight(l, "\r\n") + "\n")); err != nil {
			return
		}
	}
	if flusher != nil {
		flusher.Flush()
	}
	if !args.Follow {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case l, ok := <-lines:
			if !ok {
				return
			}
			if _, err := w.Write([]byte(strings.TrimRight(l, "\r\n") + "\n")); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}
