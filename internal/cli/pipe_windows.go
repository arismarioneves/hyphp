package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// EnvPipe troca o endereço do pipe (testes e um segundo app de desenvolvimento).
const EnvPipe = "HYPHP_PIPE"

// Address devolve o pipe do usuário atual: \\.\pipe\hyphp-<SID>. O SID no
// nome evita que dois usuários da mesma máquina disputem o mesmo pipe; o app
// é instância única por usuário, então o nome basta para a CLI achá-lo.
func Address() (string, error) {
	if p := os.Getenv(EnvPipe); p != "" {
		return p, nil
	}
	sid, err := userSID()
	if err != nil {
		return "", err
	}
	return `\\.\pipe\hyphp-` + sid, nil
}

func userSID() (string, error) {
	tok := windows.GetCurrentProcessToken()
	u, err := tok.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("cli: usuário do processo: %w", err)
	}
	return u.User.Sid.String(), nil
}

// Listen abre o pipe com acesso só para o usuário atual (DACL protegida, sem
// herança): nem outro usuário da máquina nem um serviço conseguem conectar.
func Listen(addr string) (net.Listener, error) {
	sid, err := userSID()
	if err != nil {
		return nil, err
	}
	l, err := winio.ListenPipe(addr, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;" + sid + ")"})
	if err != nil {
		return nil, fmt.Errorf("cli: abrir %s: %w", addr, err)
	}
	return l, nil
}

// Dial conecta ao pipe. Sem app aberto, o erro é ErrAppNotRunning.
func Dial(ctx context.Context, addr string) (net.Conn, error) {
	c, err := winio.DialPipeContext(ctx, addr)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrAppNotRunning
	}
	return c, err
}
