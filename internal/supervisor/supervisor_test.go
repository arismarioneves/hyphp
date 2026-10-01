package supervisor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestSupervisor(t *testing.T) *Supervisor {
	t.Helper()
	sup, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = sup.Close() })
	return sup
}

// waitState consome eventos até ver `want` para `id`. Nunca dorme às cegas.
func waitState(t *testing.T, sub <-chan Event, id string, want State, timeout time.Duration) Status {
	t.Helper()
	deadline := time.After(timeout)
	var seen []string
	for {
		select {
		case ev := <-sub:
			if ev.Status.ID != id {
				continue
			}
			seen = append(seen, string(ev.Status.State))
			if ev.Status.State == want {
				return ev.Status
			}
		case <-deadline:
			t.Fatalf("timeout de %s esperando %s ficar %q; transições vistas: %v", timeout, id, want, seen)
		}
	}
}

// closedPort devolve um endereço local sem ninguém escutando.
func closedPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func TestSupervisor_StartFicaReadyEStopMataOProcesso(t *testing.T) {
	sup := newTestSupervisor(t)
	if err := sup.Add(longRunningSpec("web")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sub, cancel := sup.Subscribe()
	defer cancel()

	if err := sup.Start("web"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitState(t, sub, "web", Starting, 2*time.Second)
	ready := waitState(t, sub, "web", Ready, 10*time.Second)
	if ready.PID == 0 {
		t.Fatal("Status.PID = 0 em ready")
	}
	if !processAlive(ready.PID) {
		t.Fatalf("processo %d não está vivo em ready", ready.PID)
	}

	if err := sup.Stop("web"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	st, _ := sup.Status("web")
	if st.State != Stopped {
		t.Fatalf("estado após Stop = %q, want %q", st.State, Stopped)
	}
	if processAlive(ready.PID) {
		t.Fatalf("processo %d continua vivo após Stop", ready.PID)
	}
}

// Dois Stops ao mesmo tempo (o watcher removendo o spec enquanto o usuário
// clica Parar): só o primeiro mata a árvore. O segundo repetia o
// TerminateJobObject com o job que o primeiro fecha, e no Windows o valor de
// um handle fechado é reciclado. O segundo também só pode voltar com o
// serviço já parado.
func TestSupervisor_StopConcorrenteParaUmaVezSo(t *testing.T) {
	sup := newTestSupervisor(t)
	if err := sup.Add(longRunningSpec("web")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sub, cancel := sup.Subscribe()
	defer cancel()
	if err := sup.Start("web"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ready := waitState(t, sub, "web", Ready, 10*time.Second)

	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	liberar := sync.OnceFunc(func() { close(release) })
	orig := stopProcessFn
	t.Cleanup(func() { stopProcessFn = orig })
	// Registrado depois do Close de newTestSupervisor, roda antes dele: um
	// teste que falhe com o primeiro Stop preso não trava o Close.
	t.Cleanup(liberar)
	stopProcessFn = func(cmd *exec.Cmd, proc procHandle, timeout time.Duration) error {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return orig(cmd, proc, timeout)
	}

	first := make(chan error, 1)
	go func() { first <- sup.Stop("web") }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("primeiro Stop não chegou a parar o processo")
	}
	second := make(chan error, 1)
	go func() { second <- sup.Stop("web") }()

	// O segundo Stop não tem como avisar que chegou à espera; meio segundo
	// sobra para ele, sem a correção, alcançar o stopProcess e voltar.
	select {
	case err := <-second:
		t.Fatalf("segundo Stop voltou (%v) com o primeiro ainda parando", err)
	case <-time.After(500 * time.Millisecond):
	}
	liberar()
	for i, ch := range []chan error{first, second} {
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("Stop %d: %v", i+1, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("Stop %d não voltou", i+1)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("stopProcess chamado %d vezes, quero 1", n)
	}
	if st, _ := sup.Status("web"); st.State != Stopped {
		t.Fatalf("estado após os dois Stops = %q, want %q", st.State, Stopped)
	}
	if processAlive(ready.PID) {
		t.Fatalf("processo %d continua vivo após Stop", ready.PID)
	}
}

// Quem mostra a lista só por eventos (a UI) precisa saber que o serviço saiu:
// o "stopped" final de Remove é igual ao de um Stop comum, e um serviço
// removido que continua na tela como parado deixa o resumo amarelo.
func TestSupervisor_RemoveAvisaQueOServicoSaiu(t *testing.T) {
	sup := newTestSupervisor(t)
	if err := sup.Add(longRunningSpec("php:8.1:0")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sub, cancel := sup.Subscribe()
	defer cancel()
	if err := sup.Start("php:8.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitState(t, sub, "php:8.1:0", Ready, 10*time.Second)

	if err := sup.Remove("php:8.1:0"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-sub:
			if ev.Status.ID != "php:8.1:0" {
				continue
			}
			if ev.Removed {
				if _, ok := sup.Status("php:8.1:0"); ok {
					t.Fatal("evento de remoção antes de o serviço sair da lista")
				}
				return
			}
		case <-deadline:
			t.Fatal("Remove não publicou evento com Removed")
		}
	}
}

// travaNoStopped segura a goroutine que loga a transição de id para
// "stopped". Dentro de Remove esse é o instante entre o Stop terminar e a
// entry sair do mapa — onde um Start concorrente achava o serviço parado.
type travaNoStopped struct {
	id     string
	armado atomic.Bool
	entrou chan struct{}
	solta  chan struct{}
	once   sync.Once
}

func (h *travaNoStopped) Enabled(context.Context, slog.Level) bool { return true }
func (h *travaNoStopped) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *travaNoStopped) WithGroup(string) slog.Handler            { return h }

func (h *travaNoStopped) Handle(_ context.Context, r slog.Record) error {
	if !h.armado.Load() {
		return nil
	}
	var id, st string
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "id":
			id = a.Value.String()
		case "state":
			st = a.Value.String()
		}
		return true
	})
	if id == h.id && st == string(Stopped) {
		h.once.Do(func() {
			close(h.entrou)
			<-h.solta
		})
	}
	return nil
}

// Start (botão, Restart, StartAll) chegando enquanto Remove para o serviço
// subia um processo numa entry que Remove apagava em seguida: vivo, fora de
// List() e com Stop respondendo "desconhecido" até o app fechar.
func TestSupervisor_StartDuranteRemoveNaoDeixaProcessoOrfao(t *testing.T) {
	const id = "php:8.1:0"
	h := &travaNoStopped{id: id, entrou: make(chan struct{}), solta: make(chan struct{})}
	soltar := sync.OnceFunc(func() { close(h.solta) })
	sup, err := New(slog.New(h))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = sup.Close() })
	// Registrado depois do Close, roda antes dele: um teste que falhe com o
	// Remove preso não trava o Close.
	t.Cleanup(soltar)
	if err := sup.Add(longRunningSpec(id)); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sub, cancel := sup.Subscribe()
	defer cancel()
	if err := sup.Start(id); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitState(t, sub, id, Ready, 10*time.Second)

	h.armado.Store(true)
	removed := make(chan error, 1)
	go func() { removed <- sup.Remove(id) }()
	select {
	case <-h.entrou:
	case <-time.After(10 * time.Second):
		t.Fatal("Remove não chegou a parar o serviço")
	}

	if err := sup.Start(id); err == nil {
		st, _ := sup.Status(id)
		_ = sup.Stop(id) // enquanto ainda alcançável, para não sobrar processo
		soltar()
		<-removed
		t.Fatalf("Start durante Remove subiu o processo %d, que sairia da lista ainda vivo", st.PID)
	} else if !errors.Is(err, ErrRemoving) {
		t.Fatalf("Start durante Remove = %v, quero ErrRemoving", err)
	}
	soltar()
	select {
	case err := <-removed:
		if err != nil {
			t.Fatalf("Remove: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Remove não voltou")
	}
	if _, ok := sup.Status(id); ok {
		t.Fatal("serviço continua na lista depois do Remove")
	}
}

// A stack troca o spec de um serviço (outra versão do Apache, outro motor de
// banco) com Replace. Remove+Add mandava o serviço para o fim da ordem, e o
// StopAll do Close passava a parar o PHP antes do web server.
func TestSupervisor_ReplaceMantemAOrdemDoStopAll(t *testing.T) {
	sup := newTestSupervisor(t)
	ids := []string{"php:8.1:0", "web:apache", "mysql"}
	for _, id := range ids {
		if err := sup.Add(longRunningSpec(id)); err != nil {
			t.Fatalf("Add %s: %v", id, err)
		}
	}
	novo := longRunningSpec("web:apache")
	novo.Name = "Apache 2.4.66"
	if err := sup.Replace(novo); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if st, _ := sup.Status("web:apache"); st.Name != novo.Name || st.State != Stopped {
		t.Fatalf("status após Replace = %+v, quero o spec novo parado", st)
	}

	sub, cancel := sup.Subscribe()
	defer cancel()
	for _, id := range ids {
		if err := sup.Start(id); err != nil {
			t.Fatalf("Start %s: %v", id, err)
		}
	}
	if err := sup.StopAll(); err != nil {
		t.Fatalf("StopAll: %v", err)
	}
	var got []string
	deadline := time.After(5 * time.Second)
	for len(got) < len(ids) {
		select {
		case ev := <-sub:
			if ev.Status.State == Stopping {
				got = append(got, ev.Status.ID)
			}
		case <-deadline:
			t.Fatalf("paradas vistas: %v", got)
		}
	}
	if want := []string{"mysql", "web:apache", "php:8.1:0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ordem do StopAll = %v, quero %v", got, want)
	}
}

func TestSupervisor_SaidaComErroSemRestartVaiParaFailed(t *testing.T) {
	sup := newTestSupervisor(t)
	spec := failingSpec("falho")
	if err := sup.Add(spec); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sub, cancel := sup.Subscribe()
	defer cancel()
	if err := sup.Start("falho"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	st := waitState(t, sub, "falho", Failed, 10*time.Second)
	if !strings.Contains(st.LastError, "código 1") {
		t.Fatalf("LastError = %q, want citando o código de saída", st.LastError)
	}
	if st.Restarts != 0 {
		t.Fatalf("Restarts = %d, want 0", st.Restarts)
	}
}

func TestSupervisor_RestartEsgotaAsTentativasEFalha(t *testing.T) {
	sup := newTestSupervisor(t)
	spec := failingSpec("falho")
	spec.Restart = RestartPolicy{
		Enabled:    true,
		MaxRetries: 2,
		BaseDelay:  50 * time.Millisecond,
		MaxDelay:   200 * time.Millisecond,
	}
	if err := sup.Add(spec); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sub, cancel := sup.Subscribe()
	defer cancel()
	if err := sup.Start("falho"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	st := waitState(t, sub, "falho", Failed, 20*time.Second)
	if st.Restarts != 2 {
		t.Fatalf("Restarts = %d, want 2", st.Restarts)
	}
}

func TestSupervisor_AddValida(t *testing.T) {
	sup := newTestSupervisor(t)
	ok := Spec{ID: "ok", Exe: "cmd.exe", Probe: AliveProbe{}}
	if err := sup.Add(ok); err != nil {
		t.Fatalf("Add válido: %v", err)
	}
	tests := []struct {
		name string
		spec Spec
		want string
	}{
		{"sem ID", Spec{Exe: "cmd.exe", Probe: AliveProbe{}}, "sem ID"},
		{"sem Exe", Spec{ID: "x", Probe: AliveProbe{}}, "Exe vazio"},
		{"sem Probe", Spec{ID: "y", Exe: "cmd.exe"}, "Probe obrigatório"},
		{"ID duplicado", ok, "ID duplicado"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := sup.Add(tc.spec)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Add() err = %v, want contendo %q", err, tc.want)
			}
		})
	}
}

func TestSupervisor_ProbeQueNuncaPassaMataOProcesso(t *testing.T) {
	sup := newTestSupervisor(t)
	spec := longRunningSpec("travado")
	spec.Probe = TCPProbe{Addr: closedPort(t)}
	spec.ProbeInterval = 100 * time.Millisecond
	spec.ProbeTimeout = 700 * time.Millisecond
	if err := sup.Add(spec); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sub, cancel := sup.Subscribe()
	defer cancel()
	if err := sup.Start("travado"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	started, _ := sup.Status("travado")
	if started.PID == 0 {
		t.Fatal("Status.PID = 0 logo após Start")
	}
	st := waitState(t, sub, "travado", Failed, 10*time.Second)
	if !strings.Contains(st.LastError, "probe não passou em 700ms") {
		t.Fatalf("LastError = %q, want citando o timeout do probe", st.LastError)
	}
	if processAlive(started.PID) {
		t.Fatalf("processo %d continua vivo após o timeout de probe", started.PID)
	}
}

func TestSupervisor_ListOrdenaPorGrupoDepoisID(t *testing.T) {
	sup := newTestSupervisor(t)
	for _, spec := range []Spec{
		{ID: "b", Group: "web", Exe: "cmd.exe", Probe: AliveProbe{}},
		{ID: "z", Group: "php", Exe: "cmd.exe", Probe: AliveProbe{}},
		{ID: "a", Group: "web", Exe: "cmd.exe", Probe: AliveProbe{}},
	} {
		if err := sup.Add(spec); err != nil {
			t.Fatalf("Add(%s): %v", spec.ID, err)
		}
	}
	var got []string
	for _, st := range sup.List() {
		got = append(got, st.Group+"/"+st.ID)
	}
	want := []string{"php/z", "web/a", "web/b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %v, want %v", got, want)
	}
}

func TestSupervisor_CloseParaTudoEEhIdempotente(t *testing.T) {
	sup := newTestSupervisor(t)
	if err := sup.Add(longRunningSpec("web")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sub, cancel := sup.Subscribe()
	defer cancel()
	if err := sup.Start("web"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ready := waitState(t, sub, "web", Ready, 10*time.Second)

	if err := sup.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if processAlive(ready.PID) {
		t.Fatalf("processo %d continua vivo após Close", ready.PID)
	}
	if err := sup.Close(); err != nil {
		t.Fatalf("Close idempotente: %v", err)
	}
	if err := sup.Start("web"); err == nil {
		t.Fatal("Start após Close deveria falhar")
	}
}

func TestSupervisor_LogsCapturaSaidaDoProcesso(t *testing.T) {
	sup := newTestSupervisor(t)
	spec := echoSpec("eco", "hyphp-linha")
	if err := sup.Add(spec); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sub, cancel := sup.Subscribe()
	defer cancel()
	if err := sup.Start("eco"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// O ring só é fechado (flush) antes da transição final, então esperar
	// Failed garante que a linha já está lá.
	waitState(t, sub, "eco", Failed, 10*time.Second)

	ring, ok := sup.Logs("eco")
	if !ok {
		t.Fatal("Logs(eco) = false")
	}
	lines := ring.Lines()
	found := false
	for _, ln := range lines {
		if strings.Contains(ln, "hyphp-linha") {
			found = true
		}
	}
	if !found {
		t.Fatalf("ring não tem a linha do processo: %v", lines)
	}
	if _, ok := sup.Logs("inexistente"); ok {
		t.Fatal("Logs(inexistente) = true")
	}
}
