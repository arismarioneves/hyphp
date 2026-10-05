//go:build !windows && !darwin

package cli

import (
	"os"
	"path/filepath"
	"strconv"
)

// Address devolve o socket unix do usuário. Linux não é empacotado; isto
// mantém o pacote compilando.
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
