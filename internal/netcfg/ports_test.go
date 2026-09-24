package netcfg

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func alwaysFree(int) bool { return true }

func TestAllocatorReserveSkipsBusyPort(t *testing.T) {
	busy := map[int]bool{9001: true}
	a := NewAllocatorWithProbe(9000, nil, func(p int) bool { return !busy[p] })

	got, err := a.Reserve("php:8.1", 4)
	if err != nil {
		t.Fatal(err)
	}
	// 9000 está livre mas 9001 não: o intervalo contíguo só começa em 9002.
	if want := []int{9002, 9003, 9004, 9005}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAllocatorReserveAvoidsOtherReservations(t *testing.T) {
	a := NewAllocatorWithProbe(9000, map[string][]int{"php:7.2": {9000, 9001, 9002, 9003}}, alwaysFree)

	got, err := a.Reserve("php:8.1", 4)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{9004, 9005, 9006, 9007}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAllocatorReserveReusesSameSize(t *testing.T) {
	probes := 0
	a := NewAllocatorWithProbe(9000, map[string][]int{"php:8.1": {9100, 9101}}, func(int) bool { probes++; return true })

	got, err := a.Reserve("php:8.1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{9100, 9101}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if probes != 0 {
		t.Fatalf("reuso não deve sondar portas; sondou %d vezes", probes)
	}
}

func TestAllocatorReserveReallocatesOnSizeChange(t *testing.T) {
	a := NewAllocatorWithProbe(9000, map[string][]int{"php:8.1": {9100, 9101}}, alwaysFree)

	got, err := a.Reserve("php:8.1", 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{9000, 9001, 9002, 9003}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if snap := a.Snapshot(); !reflect.DeepEqual(snap["php:8.1"], want) {
		t.Fatalf("snapshot desatualizado: %v", snap)
	}
}

func TestAllocatorReleaseFreesRange(t *testing.T) {
	a := NewAllocatorWithProbe(9000, nil, alwaysFree)
	if _, err := a.Reserve("php:7.2", 4); err != nil {
		t.Fatal(err)
	}
	a.Release("php:7.2")

	got, err := a.Reserve("php:8.1", 4)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{9000, 9001, 9002, 9003}; !reflect.DeepEqual(got, want) {
		t.Fatalf("após Release o intervalo deve voltar ao base: got %v", got)
	}
	if _, still := a.Snapshot()["php:7.2"]; still {
		t.Fatal("Release não removeu a chave do snapshot")
	}
}

func TestAllocatorExhausted(t *testing.T) {
	a := NewAllocatorWithProbe(65530, nil, alwaysFree)
	if _, err := a.Reserve("x", 10); err == nil {
		t.Fatal("esperava erro por falta de portas antes de 65535")
	}
}

func TestAllocatorSnapshotIsCopy(t *testing.T) {
	a := NewAllocatorWithProbe(9000, nil, alwaysFree)
	got, _ := a.Reserve("php:8.1", 2)
	got[0] = 1 // caller altera sua cópia
	if snap := a.Snapshot(); snap["php:8.1"][0] != 9000 {
		t.Fatalf("Reserve devolveu slice interno; snapshot alterado para %v", snap)
	}
}

func TestParseNetstatListening(t *testing.T) {
	fixtures := []string{"netstat_ptbr.txt", "netstat_enus.txt"}
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
