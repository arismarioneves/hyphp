package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"hyphp/internal/supervisor"
)

// LogStreamName é o nome do stream único de logs (C18.8).
const LogStreamName = "logs"

// logBacklogLines é quanto do ring vai para o cliente ao conectar.
const logBacklogLines = 500

// badHelloFrame é a resposta a um primeiro frame que não é {"id":"..."}.
const badHelloFrame = `!error: bad hello, want {"id":"<serviceID>"}`

// LogSource é uma origem de log oferecida à tela Logs.
type LogSource struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
}

// LogsDeps são as dependências injetadas por main.go.
type LogsDeps struct {
	Sup    *supervisor.Supervisor
	Logger *slog.Logger
}

// LogsService lista as origens de log. O conteúdo vai pelo stream "logs".
type LogsService struct {
	d LogsDeps
}

func NewLogsService(d LogsDeps) *LogsService {
	return &LogsService{d: d}
}

// Sources devolve um item por serviço registrado, na ordem de List().
func (l *LogsService) Sources() []LogSource {
	all := l.d.Sup.List()
	out := make([]LogSource, 0, len(all))
	for _, st := range all {
		out = append(out, LogSource{ID: st.ID, Name: st.Name, Group: st.Group})
	}
	return out
}

// parseHello lê o primeiro frame do cliente: {"id":"<serviceID>"}.
func parseHello(frame []byte) (string, error) {
	var hello struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(frame, &hello); err != nil {
		return "", fmt.Errorf("hello inválido: %w", err)
	}
	if hello.ID == "" {
		return "", errors.New("hello sem campo id")
	}
	return hello.ID, nil
}

// RegisterLogStreams registra o stream único de logs. Chamado uma vez no
// bootstrap, depois de application.New: specs nascem e morrem em runtime, então
// o nome do stream não pode carregar o ID do serviço.
func RegisterLogStreams(app *application.App, sup *supervisor.Supervisor) {
	app.HandleStream(LogStreamName, func(c *application.StreamConn) {
		serveLogStream(sup, c)
	})
}

// serveLogStream roda uma conexão: handshake, backlog do ring e stream ao vivo.
// Retornar do handler fecha a conexão (cheat-sheet §4.1).
func serveLogStream(sup *supervisor.Supervisor, c *application.StreamConn) {
	defer c.Close()

	frame, err := c.Receive()
	if err != nil {
		return
	}
	id, err := parseHello(frame)
	if err != nil {
		_ = c.Send([]byte(badHelloFrame))
		return
	}
	ring, ok := sup.Logs(id)
	if !ok {
		_ = c.Send([]byte("!error: unknown service " + id))
		return
	}

	// Assina ANTES de tirar o retrato: o inverso perderia as linhas escritas
	// entre o retrato e a assinatura. A sobreposição pode repetir uma linha,
	// que é o lado seguro do trade-off.
	lines, cancel := ring.Subscribe()
	defer cancel()

	backlog := ring.Lines()
	if len(backlog) > logBacklogLines {
		backlog = backlog[len(backlog)-logBacklogLines:]
	}
	for _, line := range backlog {
		// Send transfere a posse do slice: um []byte novo por frame.
		if err := c.Send([]byte(line)); err != nil {
			return
		}
	}

	for {
		select {
		case <-c.Context().Done():
			return
		case line, ok := <-lines:
			if !ok {
				return
			}
			if err := c.Send([]byte(line)); err != nil {
				return
			}
		}
	}
}

// ForwardServiceEvents assina o supervisor e reemite cada transição como o
// evento service:state (C12). Devolve a função que encerra a goroutine.
func ForwardServiceEvents(app *application.App, sup *supervisor.Supervisor) func() {
	events, unsubscribe := sup.Subscribe()
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case ev := <-events:
				app.Event.Emit("service:state", ev.Status)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			unsubscribe()
		})
	}
}
