package netcfg

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
)

const (
	dnsHeaderLen     = 12
	dnsMaxMessage    = 512 // mensagem DNS clássica sobre UDP; o que passar disso não nos interessa
	dnsTypeA         = 1
	dnsClassIN       = 1
	dnsTTL           = 60
	dnsRcodeNXDomain = 3
	// dnsNamePointer é o ponteiro de compressão para o offset 12 — onde a question começa.
	dnsNamePointer = 0xC00C
)

// Resolver é um servidor DNS UDP mínimo: responde A → target para o sufixo configurado e
// qualquer nome sob ele, NODATA para os outros tipos sob ele e NXDOMAIN para todo o resto. Existe só para o caso wildcard
// (spec §10.3): o arquivo hosts não aceita curinga.
type Resolver struct {
	addr   string
	suffix string // sempre minúsculo e com ponto à esquerda: ".test"
	target net.IP // sempre 4 bytes

	mu   sync.Mutex
	conn *net.UDPConn
	wg   sync.WaitGroup
}

// NewResolver cria (sem iniciar) o resolvedor. addr é "127.0.0.1:53", suffix ".test" (aceita
// "test" e "test." também), target o IPv4 devolvido nas respostas.
func NewResolver(addr, suffix string, target net.IP) *Resolver {
	return &Resolver{
		addr:   addr,
		suffix: "." + strings.Trim(strings.ToLower(strings.TrimSpace(suffix)), "."),
		target: target.To4(),
	}
}

// Start faz o bind UDP e sobe a goroutine de leitura.
func (r *Resolver) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil {
		return errors.New("resolver: já iniciado")
	}
	if r.target == nil {
		return errors.New("resolver: target precisa ser um IPv4 (o answer A carrega 4 bytes)")
	}
	udpAddr, err := net.ResolveUDPAddr("udp4", r.addr)
	if err != nil {
		return fmt.Errorf("resolver: endereço %q: %w", r.addr, err)
	}
	conn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		return fmt.Errorf("resolver: bind UDP em %s falhou: %w — use netcfg.WhoHolds(%d) para ver quem ocupa a porta (o mesmo processo costuma segurar TCP e UDP: WARP, ICS)", r.addr, err, udpAddr.Port)
	}
	r.conn = conn
	r.wg.Add(1)
	go r.serve(conn)
	return nil
}

// Stop fecha o socket e espera a goroutine sair. Idempotente; depois dele dá para dar Start de novo.
func (r *Resolver) Stop() error {
	r.mu.Lock()
	conn := r.conn
	r.conn = nil
	r.mu.Unlock()
	if conn == nil {
		return nil
	}
	err := conn.Close()
	r.wg.Wait()
	if err != nil {
		return fmt.Errorf("resolver: fechar: %w", err)
	}
	return nil
}

// Addr devolve o endereço realmente vinculado (nos testes, com porta efêmera); nil se parado.
func (r *Resolver) Addr() net.Addr {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn == nil {
		return nil
	}
	return r.conn.LocalAddr()
}

func (r *Resolver) serve(conn *net.UDPConn) {
	defer r.wg.Done()
	buf := make([]byte, dnsMaxMessage)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return // Stop()
			}
			// UDP no Windows entrega WSAECONNRESET quando um envio anterior tomou ICMP
			// port unreachable. Não é motivo para derrubar o servidor.
			continue
		}
		resp, ok := buildResponse(buf[:n], r.suffix, r.target)
		if !ok {
			continue // pacote inútil: nem responder
		}
		_, _ = conn.WriteToUDP(resp, from)
	}
}

// question é a pergunta decodificada de uma query.
type question struct {
	name  string // minúsculo, sem ponto final: "t1.hello.test"
	qtype uint16
	raw   []byte // bytes originais (nome + QTYPE + QCLASS), ecoados na resposta
}

// parseQuery valida o cabeçalho e extrai a primeira question. ok=false para pacote curto,
// mensagem que já é resposta (QR=1), opcode ≠ QUERY, QDCOUNT ≠ 1 ou nome malformado.
func parseQuery(msg []byte) (id uint16, rd bool, q question, ok bool) {
	if len(msg) < dnsHeaderLen {
		return 0, false, question{}, false
	}
	id = binary.BigEndian.Uint16(msg[0:2])
	flags := binary.BigEndian.Uint16(msg[2:4])
	if flags&0x8000 != 0 {
		return 0, false, question{}, false // é resposta, não pergunta
	}
	if (flags>>11)&0x000F != 0 {
		return 0, false, question{}, false // opcode ≠ QUERY (IQUERY, STATUS, UPDATE...)
	}
	rd = flags&0x0100 != 0
	if binary.BigEndian.Uint16(msg[4:6]) != 1 {
		return 0, false, question{}, false // QDCOUNT ≠ 1
	}
	name, off, nameOK := parseName(msg, dnsHeaderLen)
	if !nameOK || off+4 > len(msg) {
		return 0, false, question{}, false
	}
	q.name = name
	q.qtype = binary.BigEndian.Uint16(msg[off : off+2])
	q.raw = msg[dnsHeaderLen : off+4]
	return id, rd, q, true
}

// parseName lê labels a partir de off e devolve o nome em minúsculas sem ponto final e o
// offset logo após o 0x00 terminador. Rejeita label que estoura o pacote, nome > 255 bytes e
// ponteiro de compressão (0xC0): question de query real nunca usa ponteiro.
func parseName(msg []byte, off int) (string, int, bool) {
	var sb strings.Builder
	total := 0
	for {
		if off >= len(msg) {
			return "", 0, false
		}
		length := int(msg[off])
		off++
		if length == 0 {
			return strings.ToLower(sb.String()), off, true
		}
		if length&0xC0 != 0 {
			return "", 0, false // ponteiro de compressão ou label de tipo reservado
		}
		if off+length > len(msg) {
			return "", 0, false
		}
		total += length + 1
		if total > 255 {
			return "", 0, false
		}
		if sb.Len() > 0 {
			sb.WriteByte('.')
		}
		sb.Write(msg[off : off+length])
		off += length
	}
}

// buildResponse monta a resposta para msg. ok=false quando o pacote não é uma query utilizável —
// aí nada é enviado de volta (responder lixo a lixo só gera tráfego).
func buildResponse(msg []byte, suffix string, target net.IP) ([]byte, bool) {
	id, rd, q, ok := parseQuery(msg)
	if !ok {
		return nil, false
	}
	inZone := matchesSuffix(q.name, suffix)
	answer := inZone && q.qtype == dnsTypeA

	out := make([]byte, dnsHeaderLen, dnsHeaderLen+len(q.raw)+16)
	binary.BigEndian.PutUint16(out[0:2], id)
	flags := uint16(0x8000) | 0x0400 // QR=1, AA=1 (somos autoritativos no nosso sufixo), RA=0
	if rd {
		flags |= 0x0100 // RD é ecoado; não somos recursivos, então RA fica 0
	}
	var ancount uint16
	if answer {
		ancount = 1
	} else if !inZone {
		// Só fora do sufixo o nome "não existe". Dentro, AAAA e afins são
		// NODATA (NOERROR sem resposta, RFC 2308 §2.2): NXDOMAIN negaria o
		// nome inteiro e stubs com cache negativo passariam a negar o A.
		flags |= dnsRcodeNXDomain
	}
	binary.BigEndian.PutUint16(out[2:4], flags)
	binary.BigEndian.PutUint16(out[4:6], 1)       // QDCOUNT
	binary.BigEndian.PutUint16(out[6:8], ancount) // ANCOUNT; NSCOUNT e ARCOUNT ficam 0
	out = append(out, q.raw...)
	if !answer {
		return out, true
	}

	var rr [16]byte
	binary.BigEndian.PutUint16(rr[0:2], dnsNamePointer) // NAME: ponteiro para a question
	binary.BigEndian.PutUint16(rr[2:4], dnsTypeA)
	binary.BigEndian.PutUint16(rr[4:6], dnsClassIN)
	binary.BigEndian.PutUint32(rr[6:10], dnsTTL)
	binary.BigEndian.PutUint16(rr[10:12], 4) // RDLENGTH
	copy(rr[12:16], target.To4())
	return append(out, rr[:]...), true
}

// matchesSuffix: com suffix ".test", "hello.test" e "t1.hello.test" casam; "hellotest" e
// "google.com" não. O nome já vem em minúsculas e sem ponto final de parseName, então
// "HELLO.TEST." e "hello.test" caem no mesmo caso.
func matchesSuffix(name, suffix string) bool {
	return strings.HasSuffix(name, suffix)
}
