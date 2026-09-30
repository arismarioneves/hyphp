package supervisor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
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
// version 10 (0x0a). Com Version preenchida, a versão que o handshake
// anuncia também tem de bater: no Windows outro mysqld escutando em "::"
// (o do Laragon, por exemplo) atende o 127.0.0.1 da mesma porta, e só pelo
// protocolo o probe dava "pronto" para um servidor que nem é o nosso.
type MySQLProbe struct {
	Addr    string
	Version string // "8.4.11", "11.4.13"; vazio = não confere
}

const (
	mysqlProtocolV10 = 0x0a
	// Pacote de handshake: 3 bytes de tamanho, 1 de sequência, 1 de versão
	// do protocolo e a versão do servidor terminada em NUL. 256 cobre a
	// versão com folga sem ler o resto (salt, capacidades).
	mysqlHandshakeMax = 256
)

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
	if p.Version == "" {
		return nil
	}
	// O resto do pacote tem size-1 bytes (o byte do protocolo já foi lido);
	// max(…, 0) protege de um tamanho declarado 0.
	size := int(hdr[0]) | int(hdr[1])<<8 | int(hdr[2])<<16
	buf := make([]byte, min(max(size-1, 0), mysqlHandshakeMax))
	n, err := io.ReadFull(conn, buf)
	end := bytes.IndexByte(buf[:n], 0)
	if end < 0 {
		return fmt.Errorf("mysql %s: handshake without server version: %v", p.Addr, err)
	}
	if got := string(buf[:end]); !sameServerVersion(got, p.Version) {
		return fmt.Errorf("mysql %s: server on this port is %q, not %s (another server using the port?)", p.Addr, got, p.Version)
	}
	return nil
}

// sameServerVersion compara a versão do handshake com a do runtime. O MySQL
// anuncia "8.4.11"; o MariaDB, "11.4.13-MariaDB", e o 10.x ainda prefixa
// "5.5.5-" para clientes antigos ("5.5.5-10.11.19-MariaDB").
func sameServerVersion(got, want string) bool {
	got = strings.TrimPrefix(got, "5.5.5-")
	rest, ok := strings.CutPrefix(got, want)
	return ok && (rest == "" || rest[0] == '-')
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
