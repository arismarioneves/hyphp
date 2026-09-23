package supervisor

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Probe prova que um serviço está pronto. nil = pronto.
type Probe interface {
	Check(ctx context.Context) error
}

// TCPProbe passa quando consegue abrir uma conexão TCP em Addr.
type TCPProbe struct{ Addr string }

func (p TCPProbe) Check(ctx context.Context) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", p.Addr)
	if err != nil {
		return fmt.Errorf("tcp %s: %w", p.Addr, err)
	}
	return conn.Close()
}

// HTTPProbe passa quando um GET em URL recebe qualquer resposta HTTP
// (inclusive 4xx/5xx): o que importa é o servidor estar falando HTTP.
type HTTPProbe struct{ URL string }

// probeHTTPClient não segue redirects nem reaproveita conexões: cada probe é
// um GET isolado, e um 301/302 já prova que o servidor responde.
var probeHTTPClient = &http.Client{
	Transport: &http.Transport{DisableKeepAlives: true},
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func (p HTTPProbe) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return fmt.Errorf("http %s: %w", p.URL, err)
	}
	resp, err := probeHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("http %s: %w", p.URL, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return nil
}

// MySQLProbe passa quando o servidor envia um handshake com protocol
// version 10 (0x0a). Lê só os 5 primeiros bytes do pacote: 3 de tamanho,
// 1 de sequência e 1 de versão do protocolo.
type MySQLProbe struct{ Addr string }

const mysqlProtocolV10 = 0x0a

func (p MySQLProbe) Check(ctx context.Context) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", p.Addr)
	if err != nil {
		return fmt.Errorf("mysql %s: %w", p.Addr, err)
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(2 * time.Second)
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return fmt.Errorf("mysql %s: %w", p.Addr, err)
	}
	var hdr [5]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return fmt.Errorf("mysql %s: handshake: %w", p.Addr, err)
	}
	if hdr[4] != mysqlProtocolV10 {
		return fmt.Errorf("mysql %s: protocol version %#x, want %#x", p.Addr, hdr[4], mysqlProtocolV10)
	}
	return nil
}

// AliveProbe é para serviços sem porta (workers de fila): passa após Grace.
// O supervisor só chama Check enquanto o processo está vivo, então "voltou
// nil após Grace" significa "sobreviveu ao período de carência".
type AliveProbe struct{ Grace time.Duration }

func (p AliveProbe) Check(ctx context.Context) error {
	t := time.NewTimer(p.Grace)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("alive: %w", ctx.Err())
	}
}
