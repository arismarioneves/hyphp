package netcfg

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"sync"
)

// maxPort é a última porta TCP válida.
const maxPort = 65535

// ErrPortNotHeld indica que nenhum processo está em LISTENING na porta.
var ErrPortNotHeld = errors.New("nenhum processo escuta na porta")

// Allocator reserva intervalos contíguos de portas por chave ("php:8.1") e persiste via Snapshot.
type Allocator struct {
	mu       sync.Mutex
	base     int
	reserved map[string][]int
	isFree   func(port int) bool
}

// NewAllocator cria um alocador que começa em base e considera taken (de state.PortAlloc)
// como reservas já existentes. Sonda portas reais com IsFree.
func NewAllocator(base int, taken map[string][]int) *Allocator {
	return NewAllocatorWithProbe(base, taken, IsFree)
}

// NewAllocatorWithProbe é NewAllocator com a sonda de porta injetada (testes usam func(int) bool { return true }).
func NewAllocatorWithProbe(base int, taken map[string][]int, isFree func(port int) bool) *Allocator {
	a := &Allocator{base: base, reserved: make(map[string][]int, len(taken)), isFree: isFree}
	for k, ports := range taken {
		if len(ports) == 0 {
			continue
		}
		cp := make([]int, len(ports))
		copy(cp, ports)
		sort.Ints(cp)
		a.reserved[k] = cp
	}
	return a
}

// Reserve devolve n portas para key. Se key já tem exatamente n portas reservadas, devolve-as
// (sem sondar de novo). Caso contrário descarta a reserva antiga e procura n portas contíguas
// ≥ base que não estejam em nenhuma outra reserva e passem na sonda.
func (a *Allocator) Reserve(key string, n int) ([]int, error) {
	if n <= 0 {
		return nil, fmt.Errorf("reserve %q: n deve ser > 0, veio %d", key, n)
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	if cur, ok := a.reserved[key]; ok && len(cur) == n {
		out := make([]int, n)
		copy(out, cur)
		return out, nil
	}
	delete(a.reserved, key)

	used := make(map[int]struct{})
	for _, ports := range a.reserved {
		for _, p := range ports {
			used[p] = struct{}{}
		}
	}

	for start := a.base; start+n-1 <= maxPort; start++ {
		if a.rangeAvailable(start, n, used) {
			ports := make([]int, n)
			for i := range ports {
				ports[i] = start + i
			}
			a.reserved[key] = ports
			out := make([]int, n)
			copy(out, ports)
			return out, nil
		}
	}
	return nil, fmt.Errorf("reserve %q: nenhum intervalo de %d portas contíguas livres a partir de %d", key, n, a.base)
}

func (a *Allocator) rangeAvailable(start, n int, used map[int]struct{}) bool {
	for p := start; p < start+n; p++ {
		if _, taken := used[p]; taken {
			return false
		}
		if !a.isFree(p) {
			return false
		}
	}
	return true
}

// Release descarta a reserva de key. Chave desconhecida é no-op.
func (a *Allocator) Release(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.reserved, key)
}

// Snapshot devolve uma cópia das reservas, pronta para state.PortAlloc.
func (a *Allocator) Snapshot() map[string][]int {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string][]int, len(a.reserved))
	for k, ports := range a.reserved {
		cp := make([]int, len(ports))
		copy(cp, ports)
		out[k] = cp
	}
	return out
}

// IsFree devolve true se a porta TCP não está em uso. Combina dois testes porque nenhum
// sozinho é confiável no Windows: (1) bind em 127.0.0.1, 0.0.0.0 (tcp4) e [::] (tcp6) —
// o Windows permite bind específico ao lado de um listener em 0.0.0.0, e alguns servidores
// (mysqld) nem bloqueiam um segundo bind em 0.0.0.0; (2) a tabela TCP do kernel via
// `netstat -ano`, que é a fonte de verdade para LISTENING. Ordem: bind primeiro (barato).
func IsFree(port int) bool {
	if !bindable(port) {
		return false
	}
	listening, err := listeningPorts()
	if err != nil {
		return true // sem netstat, ficamos com o resultado do bind
	}
	_, held := listening[port]
	return !held
}

// bindable tenta escutar em cada endereço; qualquer falha = ocupada.
func bindable(port int) bool {
	p := strconv.Itoa(port)
	for _, probe := range [...]struct{ network, addr string }{
		{"tcp4", "127.0.0.1:" + p},
		{"tcp4", "0.0.0.0:" + p},
		{"tcp6", "[::]:" + p},
	} {
		ln, err := net.Listen(probe.network, probe.addr)
		if err != nil {
			return false
		}
		ln.Close()
	}
	return true
}

// IPv6Loopback diz se dá para escutar em ::1 nesta máquina. Falso quando o
// IPv6 foi desligado no registro (DisabledComponents).
func IPv6Loopback() bool {
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// WhoHolds identifica o processo em LISTENING TCP na porta: PID e executável vêm de
// listeningPorts/processImagePath, implementados por SO (netstat no Windows, lsof no macOS). Sem listener → ErrPortNotHeld.
// Se o PID existir mas o executável não puder ser consultado (ex.: PID 4 "System", ou
// svchost de outro usuário), devolve pid com exe vazio e err nil.
func WhoHolds(port int) (pid int, exe string, err error) {
	listening, err := listeningPorts()
	if err != nil {
		return 0, "", err
	}
	pid, ok := listening[port]
	if !ok {
		return 0, "", ErrPortNotHeld
	}
	exe, _ = processImagePath(pid)
	return pid, exe, nil
}
