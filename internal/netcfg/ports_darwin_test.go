package netcfg

import (
	"net"
	"os"
	"reflect"
	"strconv"
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

// Porta abaixo de 1024 livre não está "em uso" só porque o usuário não é
// root: o macOS recusa o bind em 127.0.0.1 e aceita o curinga, onde o Apache
// e o nginx escutam. Ocupada de verdade continua ocupada.
func TestIsFreePortaPrivilegiadaSemRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("como root todo bind passa; o caso é o usuário comum")
	}
	port := 0
	for p := 1023; p >= 600 && port == 0; p-- {
		if ln, err := net.Listen("tcp", ":"+strconv.Itoa(p)); err == nil {
			ln.Close()
			port = p
		}
	}
	if port == 0 {
		t.Fatal("nenhuma porta entre 600 e 1023 aceitou o curinga sem root")
	}
	if !IsFree(port) {
		t.Fatalf("IsFree(%d) = false com a porta livre", port)
	}
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if IsFree(port) {
		t.Fatalf("IsFree(%d) = true com a porta escutando", port)
	}
}
