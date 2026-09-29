package supervisor

import (
	"io"
	"log/slog"
	"net"
	"reflect"
	"strings"
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

func longRunningSpec(id string) Spec {
	return Spec{
		ID:    id,
		Name:  id,
		Group: "teste",
		Exe:   "cmd.exe",
		Args:  []string{"/c", "ping -n 30 127.0.0.1 >nul"},
		Probe: AliveProbe{Grace: 300 * time.Millisecond},
	}
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

func TestSupervisor_SaidaComErroSemRestartVaiParaFailed(t *testing.T) {
	sup := newTestSupervisor(t)
	spec := Spec{
		ID:    "falho",
		Exe:   "cmd.exe",
		Args:  []string{"/c", "exit 1"},
		Probe: AliveProbe{Grace: 2 * time.Second}, // nunca chega a passar
	}
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
	spec := Spec{
		ID:    "falho",
		Exe:   "cmd.exe",
		Args:  []string{"/c", "exit 1"},
		Probe: AliveProbe{Grace: 2 * time.Second},
		Restart: RestartPolicy{
			Enabled:    true,
			MaxRetries: 2,
			BaseDelay:  50 * time.Millisecond,
			MaxDelay:   200 * time.Millisecond,
		},
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
	spec := Spec{
		ID:    "eco",
		Exe:   "cmd.exe",
		Args:  []string{"/c", "echo hyphp-linha"},
		Probe: AliveProbe{Grace: 2 * time.Second},
	}
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
