//go:build !windows

package cli

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// EnvPipe troca o endereço do socket (testes e um segundo app de desenvolvimento).
const EnvPipe = "HYPHP_PIPE"

// Address devolve o socket unix do usuário. O app só é empacotado para
// Windows; isto mantém o pacote compilando nas outras plataformas.
func Address() (string, error) {
	if p := os.Getenv(EnvPipe); p != "" {
		return p, nil
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "hyphp-"+strconv.Itoa(os.Getuid())+".sock"), nil
}

// Listen abre o socket com permissão só para o dono.
func Listen(addr string) (net.Listener, error) {
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
