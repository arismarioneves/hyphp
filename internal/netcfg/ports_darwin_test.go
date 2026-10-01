package netcfg

import (
	"os"
	"reflect"
	"testing"
)

// A mesma porta em IPv4 e IPv6 pertence ao mesmo pid; o dono de cada porta
// é o processo do bloco "p" em que o "n" aparece.
func TestParseLsofListening(t *testing.T) {
	raw, err := os.ReadFile("testdata/lsof_listen.txt")
	if err != nil {
		t.Fatal(err)
	}
	got := parseLsofListening(string(raw))
	want := map[int]int{5000: 501, 7000: 501, 80: 12345, 9000: 67890}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
