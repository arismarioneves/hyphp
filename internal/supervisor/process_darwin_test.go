package supervisor

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// O Stop no macOS tem de matar a árvore inteira do serviço, não só o
// processo principal: um filho que o shell pôs em segundo plano ficaria órfão
// segurando porta e arquivo, que é o que o Job Object impede no Windows.
func TestStopProcess_MataGrupoInteiro(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	// O sh põe um sleep em segundo plano, grava o PID dele e espera.
	script := fmt.Sprintf("sleep 60 & echo $! > '%s'; wait", pidFile)
	spec := Spec{
		ID:    "teste-arvore",
		Exe:   "/bin/sh",
		Args:  []string{"-c", script},
		Probe: AliveProbe{},
	}
	cmd, h, err := startProcess(spec, NewLogRing(16))
	if err != nil {
		t.Fatalf("startProcess: %v", err)
	}
	defer closeProcessHandle(h)

	parentPid := cmd.Process.Pid
	if !processAlive(parentPid) {
		t.Fatalf("processo pai %d deveria estar vivo", parentPid)
	}

	var childPid int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil {
			if parsed, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				childPid = parsed
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if childPid == 0 {
		_ = cmd.Process.Kill()
		t.Fatal("não foi possível obter o PID do processo filho")
	}
	if !processAlive(childPid) {
		t.Fatalf("processo filho %d deveria estar vivo", childPid)
	}

	if err := stopProcess(cmd, h, 5*time.Second); err != nil {
		t.Fatalf("stopProcess: %v", err)
	}

	// cmd.Wait() só retorna depois que as goroutines de cópia de stdout/stderr
	// terminam; o filho em segundo plano herdou a ponta do pipe, então um
	// filho vivo travaria aqui.
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("cmd.Wait() bloqueou após stopProcess")
	}

	if processAlive(parentPid) {
		t.Fatalf("processo pai %d continua vivo após stopProcess", parentPid)
	}
	if processAlive(childPid) {
		t.Fatalf("processo filho %d continua vivo após stopProcess (órfão)", childPid)
	}
}

// O mysqld e o php-fpm recebem SIGTERM para fechar com ordem, mas um serviço
// que ignora o sinal não pode prender o Stop: depois do prazo vem o SIGKILL.
func TestStopProcess_SIGKILLAposGraceParaProcessoQueIgnoraSIGTERM(t *testing.T) {
	// O sh só ignora o SIGTERM depois de executar o trap; mandar o sinal logo
	// após o startProcess corria o risco de matar o shell antes disso (ação
	// padrão) e o Stop voltava antes do prazo. O script cria o arquivo pronto
	// depois do trap, e o teste espera por ele.
	pronto := filepath.Join(t.TempDir(), "pronto")
	spec := Spec{
		ID:    "teste-ignora-term",
		Exe:   "/bin/sh",
		Args:  []string{"-c", fmt.Sprintf("trap '' TERM; : > '%s'; while true; do sleep 0.1; done", pronto)},
		Probe: AliveProbe{},
	}
	cmd, h, err := startProcess(spec, NewLogRing(16))
	if err != nil {
		t.Fatalf("startProcess: %v", err)
	}
	defer closeProcessHandle(h)

	pid := cmd.Process.Pid
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(pronto); err == nil {
			break
		}
		if !time.Now().Before(deadline) {
			_ = cmd.Process.Kill()
			t.Fatal("o sh não instalou o trap a tempo")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !processAlive(pid) {
		t.Fatalf("processo %d deveria estar vivo", pid)
	}

	const grace = 500 * time.Millisecond
	start := time.Now()
	if err := stopProcess(cmd, h, grace); err != nil {
		t.Fatalf("stopProcess: %v", err)
	}
	if elapsed := time.Since(start); elapsed < grace {
		t.Fatalf("stopProcess voltou em %s, antes do prazo de %s para o SIGTERM", elapsed, grace)
	}
	if processAlive(pid) {
		t.Fatalf("processo %d continua vivo após o SIGKILL", pid)
	}
	_ = cmd.Wait()
}
