//go:build windows || darwin

package update

import (
	"io"
	"os"
	"path/filepath"
)

// copyExe copia o executável do app para o atualizador rodar de um arquivo
// que a instalação não vai substituir.
func copyExe(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
