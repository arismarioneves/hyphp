package netcfg

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const procNetTCP = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1111 1 0000000000000000 100 0 0 10 0
   1: 0100007F:1F90 0100007F:D431 01 00000000:00000000 00:00000000 00000000  1000        0 2222 1 0000000000000000 20 4 30 10 -1
   2: 0100007F:0CEA 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 3333 1 0000000000000000 100 0 0 10 0
`

const procNetTCP6 = `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000000:01BB 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 4444 1 0000000000000000 100 0 0 10 0
   1: 0000000000000000FFFF00000100007F:1F41 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 5555 1 0000000000000000 100 0 0 10 0
`

// Só socket em LISTEN entra: uma conexão estabelecida com porta local 8080
// não é um servidor na 8080. IPv6 curinga e IPv4 mapeado entram.
func TestParseProcNetTCPSoLISTEN(t *testing.T) {
	if got, want := parseProcNetTCP(procNetTCP), map[uint64]int{1111: 80, 3333: 3306}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tcp: %v; quero %v", got, want)
	}
	if got, want := parseProcNetTCP(procNetTCP6), map[uint64]int{4444: 443, 5555: 8001}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tcp6: %v; quero %v", got, want)
	}
}

// Porta em LISTEN sem dono visível (outro usuário) entra com PID 0: está
// ocupada mesmo assim. Com IPv4 e IPv6 da mesma porta, o dono visível vence.
func TestPortaDeOutroUsuarioFicaComPIDZero(t *testing.T) {
	inodePort := map[uint64]int{1: 80, 2: 3306, 3: 3306}
	inodePID := map[uint64]int{3: 4242}
	if got, want := portOwners(inodePort, inodePID), map[int]int{80: 0, 3306: 4242}; !reflect.DeepEqual(got, want) {
		t.Fatalf("portOwners = %v; quero %v", got, want)
	}
}

func TestQuemEscutaUmaPortaDoProprioTeste(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	got, err := listeningPorts()
	if err != nil {
		t.Fatal(err)
	}
	if got[port] != os.Getpid() {
		t.Fatalf("porta %d com pid %d; quero %d", port, got[port], os.Getpid())
	}
	exe, err := processImagePath(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	self, _ := os.Executable()
	if want, _ := filepath.EvalSymlinks(self); exe != want {
		t.Fatalf("processImagePath = %q; quero %q", exe, want)
	}
	if IsFree(port) {
		t.Fatalf("IsFree(%d) com o listener aberto", port)
	}
}
