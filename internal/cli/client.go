package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
)

// ErrAppNotRunning: ninguém escuta no pipe, o app está fechado.
var ErrAppNotRunning = errors.New("cli: o HyPHP não está aberto")

// RemoteError é um erro devolvido pelo app, com a mensagem no idioma dele.
type RemoteError struct {
	Status  int
	Message string
}

func (e *RemoteError) Error() string { return e.Message }

// Client fala com o app pelo pipe. Caller e Argv vão em cada pedido (ver
// HeaderCaller e HeaderArgv); Lang guarda o idioma do app visto na última
// resposta ("" antes da primeira).
type Client struct {
	Addr   string
	Caller string
	Argv   string
	Lang   string
	http   *http.Client
}

// NewClient monta um cliente para o endereço dado (Address() no uso normal).
func NewClient(addr, caller string) *Client {
	c := &Client{Addr: addr, Caller: caller}
	c.http = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return Dial(ctx, addr)
		},
		// Um pipe por chamada: a CLI faz uma ou duas e sai.
		DisableKeepAlives: true,
	}}
	return c
}

func (c *Client) post(ctx context.Context, path string, args any) (*http.Response, error) {
	body, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	// O host é simbólico: o transporte ignora e disca o pipe.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://hyphp"+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Caller != "" {
		req.Header.Set(HeaderCaller, c.Caller)
	}
	if c.Argv != "" {
		req.Header.Set(HeaderArgv, c.Argv)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, ErrAppNotRunning) {
			return nil, ErrAppNotRunning
		}
		return nil, err
	}
	if l := resp.Header.Get(HeaderLang); l != "" {
		c.Lang = l
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		var e ErrorBody
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if json.Unmarshal(raw, &e) != nil || e.Error == "" {
			e.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(raw))
		}
		return nil, &RemoteError{Status: resp.StatusCode, Message: e.Error}
	}
	return resp, nil
}

// Call roda um comando no app. out recebe o resultado (nil descarta).
func (c *Client) Call(ctx context.Context, cmd string, args, out any) error {
	if args == nil {
		args = struct{}{}
	}
	resp, err := c.post(ctx, CallPath+cmd, args)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("cli: resposta ilegível de %s: %w", cmd, err)
	}
	return nil
}

// Logs chama line para cada linha do log do serviço. Com Follow, só volta
// quando ctx acaba ou o app fecha a conexão.
func (c *Client) Logs(ctx context.Context, args LogsArgs, line func(string)) error {
	resp, err := c.post(ctx, LogsPath, args)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line(sc.Text())
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}
