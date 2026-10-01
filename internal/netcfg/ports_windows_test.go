package netcfg

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestParseNetstatListening(t *testing.T) {
	// de-DE traduz o estado ("ABHÖREN"): o listener tem de sair da coluna remota.
	fixtures := []string{"netstat_ptbr.txt", "netstat_enus.txt", "netstat_dede.txt"}
	cases := []struct {
		port    int
		wantPID int
		wantOK  bool
	}{
		{80, 18704, true},   // IPv4 0.0.0.0
		{3306, 20616, true}, // IPv4 e IPv6, mesmo PID
		{8443, 9001, true},  // só IPv6 "[::]:8443"
		{445, 4, true},      // PID 4 (System)
		{52538, 0, false},   // ESTABLISHED não conta
		{53, 6520, true},    // TCP LISTENING em 127.0.2.2 (WARP); a linha UDP 53 do PID 3872 é ignorada
		{7680, 0, false},    // TIME_WAIT não conta
		{9999, 0, false},    // ausente
	}
	for _, f := range fixtures {
		raw, err := os.ReadFile(filepath.Join("testdata", f))
		if err != nil {
			t.Fatal(err)
		}
		listening := parseNetstatListening(string(raw))
		if len(listening) != 6 {
			t.Fatalf("%s: esperava 6 portas em LISTENING (135, 445, 3306, 80, 53, 8443; v4+v6 deduplicados), veio %d: %v", f, len(listening), listening)
		}
		for _, c := range cases {
			t.Run(f+"/"+strconv.Itoa(c.port), func(t *testing.T) {
				pid, ok := listening[c.port]
				if ok != c.wantOK || pid != c.wantPID {
					t.Fatalf("port %d: got (%d,%v), want (%d,%v)", c.port, pid, ok, c.wantPID, c.wantOK)
				}
			})
		}
	}
}
