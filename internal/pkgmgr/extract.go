package pkgmgr

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// normalizeZipName converte separadores para "/" (alguns zips do Windows usam "\").
func normalizeZipName(name string) string {
	return strings.ReplaceAll(name, `\`, "/")
}

// zipRoot devolve o único diretório de primeiro nível do zip, ou "" se houver mais de
// um (zip do PHP: ext/, dev/, extras/) ou nenhum (Mailpit: só arquivos soltos).
// Arquivos soltos na raiz não contam (Apache Lounge: Apache24/ + ReadMe.txt).
func zipRoot(r *zip.Reader) string {
	root := ""
	for _, f := range r.File {
		first, _, found := strings.Cut(normalizeZipName(f.Name), "/")
		if !found {
			continue
		}
		if root == "" {
			root = first
		} else if root != first {
			return ""
		}
	}
	return root
}

// extractZip extrai r em destDir. Entradas sob strip+"/" perdem esse prefixo; as
// demais são extraídas como estão. Qualquer entrada que resolva fora de destDir
// (zip-slip) aborta a extração.
func extractZip(r *zip.Reader, destDir, strip string) error {
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	if err := os.MkdirAll(destAbs, 0o755); err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	for _, f := range r.File {
		name := normalizeZipName(f.Name)
		if strip != "" && strings.HasPrefix(name, strip+"/") {
			name = name[len(strip)+1:]
		}
		if name == "" {
			continue // a própria entrada do diretório raiz
		}
		if strings.HasPrefix(name, "/") || filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
			return fmt.Errorf("extract: entrada fora do destino (zip-slip): %q", f.Name)
		}
		target := filepath.Join(destAbs, filepath.FromSlash(name))
		rel, err := filepath.Rel(destAbs, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("extract: entrada fora do destino (zip-slip): %q", f.Name)
		}
		if f.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("extract: %w", err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("extract: %w", err)
		}
		if err := writeZipFile(f, target); err != nil {
			return err
		}
	}
	return nil
}

func writeZipFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("extract: abrir %s: %w", f.Name, err)
	}
	defer rc.Close()
	// O modo do zip preserva o bit de execução (no-op no Windows); o piso 0600
	// cobre zips gerados no Windows, que gravam modo zero.
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode().Perm()|0o600)
	if err != nil {
		return fmt.Errorf("extract: criar %s: %w", target, err)
	}
	_, copyErr := io.Copy(out, rc)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("extract: escrever %s: %w", target, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("extract: fechar %s: %w", target, closeErr)
	}
	return nil
}

// copyFile copia src para dst criando o diretório pai (usado para o .exe solto do mkcert).
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("copiar: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("copiar: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("copiar: criar %s: %w", dst, err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("copiar %s: %w", dst, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("copiar: fechar %s: %w", dst, closeErr)
	}
	return nil
}
