package supervisor

import "time"

// longRunningSpec é o par do "ping -n 30" do Windows: um sh com filhos
// (cada sleep), para o Stop ter uma árvore a matar, e que sai sozinho em
// ~30 s. Sem o limite, um teste que morresse no meio deixaria o laço vivo
// para sempre — no macOS não há job kill-on-close que o leve junto.
func longRunningSpec(id string) Spec {
	return Spec{
		ID:    id,
		Name:  id,
		Group: "teste",
		Exe:   "/bin/sh",
		Args:  []string{"-c", "i=0; while [ $i -lt 30 ]; do sleep 1; i=$((i+1)); done"},
		Probe: AliveProbe{Grace: 300 * time.Millisecond},
	}
}

// failingSpec sai com código 1 logo ao subir; o AliveProbe nunca chega a
// passar. Código 1 como no Windows: os testes conferem o código no LastError.
func failingSpec(id string) Spec {
	return Spec{
		ID:    id,
		Exe:   "/bin/sh",
		Args:  []string{"-c", "exit 1"},
		Probe: AliveProbe{Grace: 2 * time.Second},
	}
}

// echoSpec escreve text numa linha e sai.
func echoSpec(id, text string) Spec {
	return Spec{
		ID:    id,
		Exe:   "/bin/sh",
		Args:  []string{"-c", "echo " + text},
		Probe: AliveProbe{Grace: 2 * time.Second},
	}
}

// processAlive responde se o PID ainda está em execução; zumbi conta como
// morto (ver exited em process_darwin.go).
func processAlive(pid int) bool {
	return !exited(pid)
}
