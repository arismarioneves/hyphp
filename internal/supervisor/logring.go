package supervisor

import (
	"bytes"
	"sync"
)

// subscriberBuffer é o tamanho do canal de cada assinante. Um assinante que
// não consome a tempo perde linhas (contadas em Dropped); o processo nunca
// fica bloqueado escrevendo em stdout por causa da UI.
const subscriberBuffer = 256

// maxPartialBytes limita a linha parcial: saída binária sem '\n' é fechada
// à força para não crescer sem limite.
const maxPartialBytes = 64 << 10

type lineSubscriber struct {
	ch chan string
}

// LogRing guarda as últimas `capacity` linhas de um processo e replica linhas
// novas para assinantes sem bloquear o escritor.
type LogRing struct {
	mu       sync.Mutex
	capacity int
	lines    []string // armazenamento circular, len == capacity
	head     int      // índice da próxima escrita
	count    int      // linhas válidas (<= capacity)
	partial  []byte   // bytes após o último '\n', ainda sem linha fechada
	subs     map[*lineSubscriber]struct{}
	dropped  int
}

func NewLogRing(capacity int) *LogRing {
	if capacity < 1 {
		capacity = 1
	}
	return &LogRing{
		capacity: capacity,
		lines:    make([]string, capacity),
		subs:     make(map[*lineSubscriber]struct{}),
	}
}

// Write implementa io.Writer. Cada '\n' fecha uma linha ('\r' final removido).
// Bytes sem '\n' ficam pendentes até o próximo Write ou flush.
func (r *LogRing) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rest := p
	for {
		i := bytes.IndexByte(rest, '\n')
		if i < 0 {
			r.partial = append(r.partial, rest...)
			if len(r.partial) >= maxPartialBytes {
				r.pushPartialLocked()
			}
			return len(p), nil
		}
		r.partial = append(r.partial, rest[:i]...)
		r.pushPartialLocked()
		rest = rest[i+1:]
	}
}

// flush fecha a linha parcial pendente. Chamado pelo supervisor quando o
// processo encerra, para a última linha sem '\n' não se perder.
func (r *LogRing) flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.partial) > 0 {
		r.pushPartialLocked()
	}
}

func (r *LogRing) pushPartialLocked() {
	line := decodeLine(bytes.TrimSuffix(r.partial, []byte{'\r'}))
	r.partial = r.partial[:0]
	r.lines[r.head] = line
	r.head = (r.head + 1) % r.capacity
	if r.count < r.capacity {
		r.count++
	}
	for s := range r.subs {
		select {
		case s.ch <- line:
		default:
			r.dropped++
		}
	}
}

// Lines devolve uma cópia das linhas retidas, da mais antiga à mais nova.
func (r *LogRing) Lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, r.count)
	start := (r.head - r.count + r.capacity) % r.capacity
	for i := range r.count {
		out[i] = r.lines[(start+i)%r.capacity]
	}
	return out
}

// Subscribe devolve um canal que recebe cada linha nova e uma função de
// cancelamento idempotente que fecha o canal. Linhas já retidas não são
// replicadas: quem quiser histórico chama Lines() antes.
func (r *LogRing) Subscribe() (<-chan string, func()) {
	s := &lineSubscriber{ch: make(chan string, subscriberBuffer)}
	r.mu.Lock()
	r.subs[s] = struct{}{}
	r.mu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.subs, s) // após isto nenhum Write envia para s.ch
			r.mu.Unlock()
			close(s.ch)
		})
	}
	return s.ch, cancel
}

// Dropped devolve o total de linhas descartadas por assinantes lentos.
func (r *LogRing) Dropped() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}
