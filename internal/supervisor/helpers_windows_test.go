package supervisor

import (
	"time"

	"golang.org/x/sys/windows"
)

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

// failingSpec sai com código 1 logo ao subir; o AliveProbe nunca chega a
// passar.
func failingSpec(id string) Spec {
	return Spec{
		ID:    id,
		Exe:   "cmd.exe",
		Args:  []string{"/c", "exit 1"},
		Probe: AliveProbe{Grace: 2 * time.Second},
	}
}

// echoSpec escreve text numa linha e sai.
func echoSpec(id, text string) Spec {
	return Spec{
		ID:    id,
		Exe:   "cmd.exe",
		Args:  []string{"/c", "echo " + text},
		Probe: AliveProbe{Grace: 2 * time.Second},
	}
}

const stillActive = 259 // STILL_ACTIVE

// processAlive responde se o PID ainda está em execução. Usado pelos testes
// de parada e pelo supervisor_test.go.
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
