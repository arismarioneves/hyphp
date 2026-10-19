//go:build darwin || linux

package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"

	"hyphp/internal/paths"
)

// EnvPipe troca o endereço do socket (testes e um segundo app de desenvolvimento).
const EnvPipe = "HYPHP_PIPE"

// Address devolve o socket da CLI dentro da raiz do HyPHP. App e CLI calculam
// paths.Run() do mesmo jeito (home + HYPHP_ROOT), então concordam sem
// depender do TMPDIR ou do XDG_RUNTIME_DIR, que não existem em sessões ssh e
// sudo.
func Address() (string, error) {
	if p := os.Getenv(EnvPipe); p != "" {
		return p, nil
	}
	addr := filepath.Join(paths.Run(), "cli.sock")
	// sun_path tem 104 bytes no macOS e 108 no Linux, com o NUL final; o
	// limite menor vale para os dois.
	if len(addr) > 103 {
		return "", fmt.Errorf("cli: caminho do socket longo demais (%d bytes): %s", len(addr), addr)
	}
	return addr, nil
}

// Listen abre o socket com permissão só para o dono.
func Listen(addr string) (net.Listener, error) {
	// A pasta run fica na raiz do HyPHP; o boot já a cria, mas a CLI de teste
	// pode escutar antes disso.
	if err := os.MkdirAll(filepath.Dir(addr), 0o700); err != nil {
		return nil, err
	}
	_ = os.Remove(addr) // socket de uma execução que não fechou direito
	l, err := net.Listen("unix", addr)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(addr, 0o600); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

// Dial conecta ao socket. Sem app aberto, o erro é ErrAppNotRunning.
func Dial(ctx context.Context, addr string) (net.Conn, error) {
	var d net.Dialer
	c, err := d.DialContext(ctx, "unix", addr)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) {
		return nil, ErrAppNotRunning
	}
	return c, err
}
