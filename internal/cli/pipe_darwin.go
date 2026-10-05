package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"hyphp/internal/paths"
)

// Address devolve o socket da CLI dentro da raiz do HyPHP. App e CLI calculam
// paths.Run() do mesmo jeito (home + HYPHP_ROOT), então concordam sem
// depender do TMPDIR, que não existe em sessões ssh e sudo.
func Address() (string, error) {
	if p := os.Getenv(EnvPipe); p != "" {
		return p, nil
	}
	addr := filepath.Join(paths.Run(), "cli.sock")
	// sun_path no macOS tem 104 bytes, com o NUL final.
	if len(addr) > 103 {
		return "", fmt.Errorf("cli: caminho do socket longo demais (%d bytes): %s", len(addr), addr)
	}
	return addr, nil
}
