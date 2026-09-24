package netcfg

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// dnsHeader monta os 12 bytes de cabeçalho de uma query: RD=1, QDCOUNT=1, resto zerado.
func dnsHeader(id uint16) []byte {
	h := make([]byte, 12)
	binary.BigEndian.PutUint16(h[0:2], id)
	binary.BigEndian.PutUint16(h[2:4], 0x0100) // QR=0, opcode=0, RD=1
	binary.BigEndian.PutUint16(h[4:6], 1)      // QDCOUNT
	return h
}

// dnsQuery monta uma query completa byte a byte: cabeçalho + labels + QTYPE + QCLASS(IN).
func dnsQuery(t *testing.T, id uint16, name string, qtype uint16) []byte {
	t.Helper()
	msg := dnsHeader(id)
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if label == "" || len(label) > 63 {
			t.Fatalf("nome inválido para o teste: %q", name)
		}
		msg = append(msg, byte(len(label)))
		msg = append(msg, label...)
	}
	msg = append(msg, 0) // fim do nome
	tail := make([]byte, 4)
	binary.BigEndian.PutUint16(tail[0:2], qtype)
	binary.BigEndian.PutUint16(tail[2:4], 1) // CLASS=IN
	return append(msg, tail...)
}

// dialResolver sobe um resolvedor em porta efêmera e devolve um socket já conectado a ele.
func dialResolver(t *testing.T) (*Resolver, net.Conn) {
	t.Helper()
	r := NewResolver("127.0.0.1:0", ".test", net.IPv4(127, 0, 0, 1))
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := r.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
	if r.Addr() == nil {
		t.Fatal("Addr() devolveu nil com o resolvedor no ar")
	}
	conn, err := net.Dial("udp4", r.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return r, conn
}

func TestResolverResponde(t *testing.T) {
	_, conn := dialResolver(t)

	tests := []struct {
		name      string
		qname     string
		qtype     uint16
		wantRcode uint16
		wantIP    string
	}{
		{"A no domínio", "hello.test", 1, 0, "127.0.0.1"},
		{"A em subdomínio", "t1.hello.test", 1, 0, "127.0.0.1"},
		{"A em subdomínio de subdomínio", "a.b.hello.test", 1, 0, "127.0.0.1"},
		{"A com ponto final", "hello.test.", 1, 0, "127.0.0.1"},
		{"A em maiúsculas", "HELLO.TEST", 1, 0, "127.0.0.1"},
		{"A fora do sufixo", "google.com", 1, 3, ""},
		{"A em nome que só parece o sufixo", "hellotest", 1, 3, ""},
		{"AAAA no domínio", "hello.test", 28, 3, ""},
		{"MX no domínio", "hello.test", 15, 3, ""},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := uint16(0x1000 + i)
			query := dnsQuery(t, id, tt.qname, tt.qtype)
			if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Write(query); err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, 512)
			n, err := conn.Read(buf)
			if err != nil {
				t.Fatalf("sem resposta: %v", err)
			}
			resp := buf[:n]

			if got := binary.BigEndian.Uint16(resp[0:2]); got != id {
				t.Fatalf("ID: got %#x, want %#x", got, id)
			}
			flags := binary.BigEndian.Uint16(resp[2:4])
			if flags&0x8000 == 0 {
				t.Error("QR deveria ser 1")
			}
			if flags&0x0400 == 0 {
				t.Error("AA deveria ser 1")
			}
			if flags&0x0100 == 0 {
				t.Error("RD deveria ter sido ecoado")
			}
			if flags&0x0080 != 0 {
				t.Error("RA deveria ser 0")
			}
			if got := flags & 0x000F; got != tt.wantRcode {
				t.Fatalf("RCODE: got %d, want %d", got, tt.wantRcode)
			}
			if !bytes.Equal(resp[12:len(query)], query[12:]) {
				t.Fatal("a question não foi ecoada byte a byte")
			}

			if tt.wantIP == "" {
				if got := binary.BigEndian.Uint16(resp[6:8]); got != 0 {
					t.Fatalf("ANCOUNT: got %d, want 0", got)
				}
				if n != len(query) {
					t.Fatalf("resposta sem answer deveria ter %d bytes, veio %d", len(query), n)
				}
				return
			}
			if got := binary.BigEndian.Uint16(resp[6:8]); got != 1 {
				t.Fatalf("ANCOUNT: got %d, want 1", got)
			}
			rr := resp[len(query):]
			if len(rr) != 16 {
				t.Fatalf("answer deveria ter 16 bytes, veio %d", len(rr))
			}
			if got := binary.BigEndian.Uint16(rr[0:2]); got != 0xC00C {
				t.Fatalf("ponteiro de compressão: got %#x, want 0xc00c", got)
			}
			if got := binary.BigEndian.Uint16(rr[2:4]); got != 1 {
				t.Fatalf("TYPE: got %d, want 1 (A)", got)
			}
			if got := binary.BigEndian.Uint16(rr[4:6]); got != 1 {
				t.Fatalf("CLASS: got %d, want 1 (IN)", got)
			}
			if got := binary.BigEndian.Uint32(rr[6:10]); got != 60 {
				t.Fatalf("TTL: got %d, want 60", got)
			}
			if got := binary.BigEndian.Uint16(rr[10:12]); got != 4 {
				t.Fatalf("RDLENGTH: got %d, want 4", got)
			}
			if got := net.IP(rr[12:16]).String(); got != tt.wantIP {
				t.Fatalf("IP: got %s, want %s", got, tt.wantIP)
			}
		})
	}
}

func TestResolverIgnoraPacoteInvalido(t *testing.T) {
	_, conn := dialResolver(t)

	bad := [][]byte{
		{},
		{0x12},
		{0x12, 0x34, 0x01, 0x00, 0x00, 0x01}, // cabeçalho incompleto
		dnsHeader(0x2001),                    // cabeçalho sem question
		append(dnsHeader(0x2002), 0x05, 'a', 'b'),                     // label diz 5 bytes, vieram 2
		append(dnsHeader(0x2003), 0xC0, 0x0C, 0x00, 0x01, 0x00, 0x01), // ponteiro de compressão na question
		append(dnsHeader(0x2004), 0x04, 't', 'e', 's', 't', 0x00),     // nome ok, sem QTYPE/QCLASS
	}
	for i, pkt := range bad {
		if err := conn.SetDeadline(time.Now().Add(300 * time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		if len(pkt) > 0 {
			if _, err := conn.Write(pkt); err != nil {
				t.Fatal(err)
			}
		}
		buf := make([]byte, 512)
		if _, err := conn.Read(buf); err == nil {
			t.Fatalf("caso %d: resolvedor respondeu a pacote inválido % x", i, pkt)
		} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
			t.Fatalf("caso %d: erro inesperado: %v", i, err)
		}
	}

	// O servidor continua vivo depois do lixo.
	query := dnsQuery(t, 0x2100, "hello.test", 1)
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(query); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 512)
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("resolvedor morreu depois de receber lixo: %v", err)
	}
	if got := binary.BigEndian.Uint16(buf[6:8]); got != 1 {
		t.Fatalf("ANCOUNT após o lixo: got %d, want 1", got)
	}
}

func TestResolverPortaOcupada(t *testing.T) {
	r1, _ := dialResolver(t)

	r2 := NewResolver(r1.Addr().String(), ".test", net.IPv4(127, 0, 0, 1))
	err := r2.Start()
	if err == nil {
		r2.Stop()
		t.Fatal("esperava erro de bind na porta já ocupada")
	}
	if !strings.Contains(err.Error(), "bind UDP") || !strings.Contains(err.Error(), "WhoHolds") {
		t.Fatalf("erro deveria explicar o bind e apontar o WhoHolds: %v", err)
	}
	if r2.Addr() != nil {
		t.Fatal("Addr() deveria continuar nil depois de um Start que falhou")
	}
}

func TestResolverCicloDeVida(t *testing.T) {
	r := NewResolver("127.0.0.1:0", ".test", net.IPv4(127, 0, 0, 1))
	if err := r.Stop(); err != nil {
		t.Fatalf("Stop sem Start deveria ser no-op: %v", err)
	}
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	if err := r.Start(); err == nil {
		t.Fatal("segundo Start deveria falhar")
	}
	if err := r.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(); err != nil {
		t.Fatalf("segundo Stop deveria ser no-op: %v", err)
	}
	if r.Addr() != nil {
		t.Fatal("Addr() deveria ser nil depois de Stop")
	}
	if err := r.Start(); err != nil {
		t.Fatalf("Start depois de Stop deveria funcionar: %v", err)
	}
	if err := r.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestResolverAlvoPrecisaSerIPv4(t *testing.T) {
	r := NewResolver("127.0.0.1:0", ".test", net.ParseIP("::1"))
	if err := r.Start(); err == nil {
		r.Stop()
		t.Fatal("esperava erro: o answer A só carrega 4 bytes")
	}
}
