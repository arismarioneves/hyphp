package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"hyphp/internal/paths"
)

// App e CLI precisam achar o mesmo socket sem depender do TMPDIR, que muda
// entre o app aberto pelo Finder e uma sessão ssh/sudo.
func TestSocketNaPastaRunDoHyPHP(t *testing.T) {
	// Raiz curta: o t.TempDir() do macOS já estoura os 103 bytes do socket.
	root := "/tmp/hyphp-teste"
	t.Setenv(paths.EnvRoot, root)
	t.Setenv(EnvPipe, "")
	got, err := Address()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "var", "run", "cli.sock"); got != want {
		t.Fatalf("Address() = %q, want %q", got, want)
	}
}

// O sun_path do macOS tem 104 bytes; passar disso falha no bind com um erro
// obscuro, então o Address recusa antes, dizendo o motivo.
func TestSocketLongoDemaisRecusado(t *testing.T) {
	t.Setenv(paths.EnvRoot, "/"+strings.Repeat("x", 120))
	t.Setenv(EnvPipe, "")
	if _, err := Address(); err == nil {
		t.Fatal("caminho de socket com mais de 103 bytes aceito")
	}
}
