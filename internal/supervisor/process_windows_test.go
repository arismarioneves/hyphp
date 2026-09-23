package supervisor

import (
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const stillActive = 259 // STILL_ACTIVE

// processAlive responde se o PID ainda está em execução. Usado também pelo
// supervisor_test.go (Task 7).
func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

func TestStopProcess_MataArvoreEDesbloqueiaWait(t *testing.T) {
	spec := Spec{
		ID:    "teste",
		Exe:   "cmd.exe",
		Args:  []string{"/c", "ping -n 30 127.0.0.1 >nul"},
		Probe: AliveProbe{},
	}
	cmd, job, err := startProcess(spec, NewLogRing(16))
	if err != nil {
		t.Fatalf("startProcess: %v", err)
	}
	pid := cmd.Process.Pid
	if !processAlive(pid) {
		t.Fatalf("processo %d deveria estar vivo logo após startProcess", pid)
	}

	start := time.Now()
	if err := stopProcess(cmd, job, 5*time.Second); err != nil {
		t.Fatalf("stopProcess: %v", err)
	}
	if el := time.Since(start); el > 5*time.Second {
		t.Fatalf("stopProcess levou %s", el)
	}

	// cmd.Wait() só retorna depois que as goroutines de cópia de stdout/stderr
	// terminam; se um descendente segurasse a ponta do pipe, travaria aqui.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cmd.Wait() bloqueou após TerminateJobObject")
	}

	if processAlive(pid) {
		t.Fatalf("processo %d continua vivo após stopProcess", pid)
	}
	windows.CloseHandle(job)
}

// Prova a razão de stopProcess não usar cmd.Wait(): a segunda chamada volta
// na hora com erro, então não serviria de espera com timeout.
func TestSegundoWaitVoltaNaHora(t *testing.T) {
	spec := Spec{ID: "teste", Exe: "cmd.exe", Args: []string{"/c", "exit 0"}, Probe: AliveProbe{}}
	cmd, job, err := startProcess(spec, NewLogRing(4))
	if err != nil {
		t.Fatalf("startProcess: %v", err)
	}
	defer windows.CloseHandle(job)
	if err := cmd.Wait(); err != nil {
		t.Fatalf("primeiro Wait: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("segundo Wait devolveu nil; esperava erro")
	} else {
		t.Logf("segundo Wait: %v", err)
	}
}
