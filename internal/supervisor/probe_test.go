package supervisor

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func listenLocal(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

// closedAddr devolve um endereço local que acabou de deixar de escutar.
func closedAddr(t *testing.T) string {
	t.Helper()
	ln := listenLocal(t)
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func TestTCPProbe(t *testing.T) {
	open := listenLocal(t)
	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{"porta aberta", open.Addr().String(), false},
		{"porta fechada", closedAddr(t), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := TCPProbe{Addr: tc.addr}.Check(testCtx(t))
			if (err != nil) != tc.wantErr {
				t.Fatalf("Check() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestHTTPProbe(t *testing.T) {
	srv500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv500.Close)
	srv302 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1/nunca", http.StatusFound)
	}))
	t.Cleanup(srv302.Close)

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"500 conta como pronto", srv500.URL + "/", false},
		{"302 não é seguido e conta como pronto", srv302.URL + "/", false},
		{"servidor ausente", "http://" + closedAddr(t) + "/", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := HTTPProbe{URL: tc.url}.Check(testCtx(t))
			if (err != nil) != tc.wantErr {
				t.Fatalf("Check() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// fakeMySQL aceita conexões e escreve payload em cada uma, como um mysqld
// que envia o handshake inicial antes de qualquer byte do cliente.
func fakeMySQL(t *testing.T, payload []byte) string {
	t.Helper()
	ln := listenLocal(t)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = conn.Write(payload)
			conn.Close()
		}
	}()
	return ln.Addr().String()
}

func TestMySQLProbe(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		wantErr string // "" = sucesso
	}{
		{"handshake protocolo 10", []byte{0x4a, 0x00, 0x00, 0x00, 0x0a}, ""},
		{"protocolo errado", []byte{0x4a, 0x00, 0x00, 0x00, 0x09}, "protocol version 0x9"},
		{"pacote curto", []byte{0x4a, 0x00}, "handshake"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := MySQLProbe{Addr: fakeMySQL(t, tc.payload)}.Check(testCtx(t))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Check() err = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Check() err = %v, want contendo %q", err, tc.wantErr)
			}
		})
	}
}

func TestAliveProbe(t *testing.T) {
	t.Run("passa após Grace", func(t *testing.T) {
		start := time.Now()
		if err := (AliveProbe{Grace: 50 * time.Millisecond}).Check(testCtx(t)); err != nil {
			t.Fatalf("Check() err = %v", err)
		}
		if el := time.Since(start); el < 50*time.Millisecond {
			t.Fatalf("voltou antes de Grace: %s", el)
		}
	})
	t.Run("ctx cancelado interrompe", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := (AliveProbe{Grace: time.Hour}).Check(ctx); err == nil {
			t.Fatal("esperava erro de contexto")
		}
	})
}
