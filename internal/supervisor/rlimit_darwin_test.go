package supervisor

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// Garante que o filho não volta ao soft 256 do launchd depois de
// RaiseFileLimit (o os/exec restauraria o limite original sem ela).
func TestRaiseFileLimitChildInherits(t *testing.T) {
	if err := RaiseFileLimit(); err != nil {
		t.Fatalf("RaiseFileLimit: %v", err)
	}
	out, err := exec.Command("/bin/sh", "-c", "ulimit -n").Output()
	if err != nil {
		t.Fatalf("ulimit -n: %v", err)
	}
	s := strings.TrimSpace(string(out))
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
