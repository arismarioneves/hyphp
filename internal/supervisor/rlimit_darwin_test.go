package supervisor

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// childNofile devolve o `ulimit -n` visto por um filho do os/exec.
func childNofile(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("/bin/sh", "-c", "ulimit -n").Output()
	if err != nil {
		t.Fatalf("ulimit -n: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// Simula um app aberto pelo Finder (soft 256) independentemente do limite do
// runner do CI: sem isso o soft do runner já poderia ser alto e o teste
// passaria mesmo sem RaiseFileLimit.
func TestRaiseFileLimitChildInherits(t *testing.T) {
	var saved syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &saved); err != nil {
		t.Fatalf("Getrlimit: %v", err)
	}
	t.Cleanup(func() {
		if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &saved); err != nil {
			t.Errorf("restaurar RLIMIT_NOFILE: %v", err)
		}
	})
	low := saved
	low.Cur = 256
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &low); err != nil {
		t.Fatalf("baixar soft para 256: %v", err)
	}
	// prova o cenário: o filho herda os 256
	if got := childNofile(t); got != "256" {
		t.Fatalf("setup: filho ve %q, quer 256", got)
	}

	if err := RaiseFileLimit(); err != nil {
		t.Fatalf("RaiseFileLimit: %v", err)
	}
	s := childNofile(t)
	if s == "unlimited" {
		return
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("saida inesperada %q: %v", s, err)
	}
	if n <= 256 {
		t.Fatalf("limite do filho = %d, quer > 256", n)
	}
}
