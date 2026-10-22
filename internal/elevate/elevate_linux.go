package elevate

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"hyphp/internal/i18n"
)

// Códigos de saída do próprio pkexec. O helper usa só 0, 2 e 3.
const (
	pkexecDismissed = 126 // a janela de senha foi fechada ou cancelada
	pkexecFailed    = 127 // sem autorização, sem agente para pedir a senha, ou erro
)

// RunElevated roda exe com args como root pelo pkexec, que pede a senha pelo
// agente do polkit da sessão (a janela do GNOME, do Cinnamon, do KDE). A
// mensagem da janela vem da action do polkit do HyPHP, quando instalada.
//
// Erros: ErrElevationDenied (janela cancelada), ErrHelperTimeout (a janela de
// senha ficou aberta 2 min), *HelperError (o helper saiu com código ≠ 0) e os
// do pkexec.
func RunElevated(exe string, args []string) error {
	return runElevated(exe, args, helperTimeout)
}

// runElevated é o RunElevated com o prazo da janela de senha como parâmetro,
// que o teste encurta.
func runElevated(exe string, args []string, timeout time.Duration) error {
	pkexec, err := exec.LookPath("pkexec")
	if err != nil {
		return i18n.Errorf("err.elevate.noPkexec")
	}
	var stdout, stderr bytes.Buffer
	// --disable-internal-agent: sem agente na sessão, o pkexec cairia num
	// pedido de senha pelo terminal, que o app não tem; assim ele falha logo.
	cmd := exec.Command(pkexec, append([]string{"--disable-internal-agent", exe}, args...)...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		return &HelperError{ExitCode: -1, Message: err.Error()}
	}
	// O prazo limita a espera pela janela de senha, quando o pkexec ainda roda
	// com o usuário como dono e o Kill o encerra. Depois da senha ele vira o
	// próprio helper, como root: o Kill falha, e o Wait espera o helper
	// terminar, porque o resultado dele vale mesmo depois do prazo. Fechar a
	// saída antes disso (o WaitDelay do exec) mataria o helper com SIGPIPE ao
	// escrever o resultado.
	var expirou atomic.Bool
	timer := time.AfterFunc(timeout, func() {
		if cmd.Process.Kill() == nil {
			expirou.Store(true)
		}
	})
	err = cmd.Wait()
	timer.Stop()
	st := cmd.ProcessState
	if st == nil || !st.Exited() {
		if expirou.Load() {
			return ErrHelperTimeout
		}
		return &HelperError{ExitCode: -1, Message: err.Error()}
	}
	switch code := st.ExitCode(); code {
	case 0:
		return nil
	case pkexecDismissed:
		return ErrElevationDenied
	case pkexecFailed:
		return i18n.Errorf("err.elevate.pkexecFailed", strings.TrimSpace(stderr.String()))
	default:
		// Sem a linha JSON (um panic do helper, por exemplo), o motivo está no
		// stderr.
		out := stdout.String()
		if strings.TrimSpace(out) == "" {
			out = stderr.String()
		}
		return helperFailure(code, out)
	}
}

// HelperPath devolve o hyphp-helper ao lado do executável em curso: bin/ no
// build local, a pasta do app no pacote.
func HelperPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("HelperPath: %w", err)
	}
	return helperPathIn(filepath.Dir(exe))
}

func helperPathIn(dir string) (string, error) {
	p := filepath.Join(dir, "hyphp-helper")
	st, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("HelperPath: %s ausente (rode `wails3 task linux:build:helper`): %w", p, err)
	}
	if st.IsDir() {
		return "", fmt.Errorf("HelperPath: %s é um diretório", p)
	}
	return p, nil
}
