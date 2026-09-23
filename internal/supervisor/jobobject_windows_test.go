package supervisor

import (
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Prova a semântica que o Stop depende: fechar o último handle de um job
// criado por newKillOnCloseJob mata o processo associado. Um bug plausível
// (esquecer LimitFlags, tamanho errado em SetInformationJobObject) passa
// silencioso em tudo o mais e falha aqui.
func TestNewKillOnCloseJob_ClosingHandleKillsProcess(t *testing.T) {
	job, err := newKillOnCloseJob()
	if err != nil {
		t.Fatalf("newKillOnCloseJob: %v", err)
	}
	cmd := exec.Command("cmd.exe", "/c", "ping -n 30 127.0.0.1 >nul")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := assignPID(job, cmd.Process.Pid); err != nil {
		cmd.Process.Kill()
		t.Fatalf("assignPID: %v", err)
	}
	if err := windows.CloseHandle(job); err != nil {
		t.Fatalf("CloseHandle: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		// morreu: é o que KILL_ON_JOB_CLOSE promete
	case <-time.After(3 * time.Second):
		cmd.Process.Kill()
		t.Fatal("processo continuou vivo 3s após fechar o job")
	}
}

func TestAttachSelfToKillOnCloseJob_IsIdempotent(t *testing.T) {
	h1, err := AttachSelfToKillOnCloseJob()
	if err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}
	h2, err := AttachSelfToKillOnCloseJob()
	if err != nil {
		t.Fatalf("segunda chamada: %v", err)
	}
	if h1 == 0 || h1 != h2 {
		t.Fatalf("handles = %v, %v; want iguais e não nulos", h1, h2)
	}
}
