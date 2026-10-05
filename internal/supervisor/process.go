package supervisor

import (
	"errors"
	"fmt"
	"os/exec"
)

// O controle de processos é por SO (process_windows.go, process_darwin.go),
// e cada um fornece o mesmo conjunto, para supervisor.go não conhecer job
// nem grupo de processos:
//
//	type procHandle struct{ ... }  // o que alcança a árvore do serviço
//	func attachSelf() (procHandle, error)
//	func startProcess(spec Spec, out io.Writer) (*exec.Cmd, procHandle, error)
//	func stopProcess(cmd *exec.Cmd, h procHandle, timeout time.Duration) error
//	func closeProcessHandle(h procHandle)
//	func decodeLine(b []byte) string  // decode_<so>.go
//
// stopProcess nunca chama cmd.Wait(): o laço run() é o único dono dessa
// chamada. A limpeza de órfãos segue o mesmo esquema por SO (ver orphans.go).

// exitReason traduz o erro de cmd.Wait() na causa que vai para Status.LastError.
func exitReason(waitErr error) error {
	if waitErr == nil {
		return errors.New("processo encerrou com código 0")
	}
	var ee *exec.ExitError
	if errors.As(waitErr, &ee) {
		return fmt.Errorf("processo encerrou com código %d", ee.ExitCode())
	}
	return fmt.Errorf("espera do processo: %w", waitErr)
}
