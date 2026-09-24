package pkgmgr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// progressStep é o intervalo de bytes entre chamadas de onProgress.
const progressStep = 256 * 1024

// countingWriter chama onProgress a cada progressStep bytes e no último byte.
type countingWriter struct {
	n, last, total int64
	onProgress     func(done, total int64)
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	if c.n-c.last >= progressStep || c.n == c.total {
		c.last = c.n
		c.onProgress(c.n, c.total)
	}
	return len(p), nil
}

// download baixa url para dst (criado/truncado) em streaming, reportando progresso
// (total = -1 quando o servidor não informa Content-Length) e devolvendo o SHA-256
// hex minúsculo do conteúdo. Respostas text/html são rejeitadas: Apache Lounge
// responde 200 + HTML "Oops" quando uma build foi substituída.
func download(ctx context.Context, client *http.Client, url, dst string, onProgress func(done, total int64)) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "hyphp/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %s", url, resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
		return "", fmt.Errorf("download %s: servidor devolveu %s em vez do arquivo (link movido?)", url, ct)
	}

	f, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("download: criar %s: %w", dst, err)
	}
	h := sha256.New()
	cw := &countingWriter{total: resp.ContentLength, onProgress: onProgress}
	_, copyErr := io.Copy(io.MultiWriter(f, h, cw), resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return "", fmt.Errorf("download %s: %w", url, copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("download: fechar %s: %w", dst, closeErr)
	}
	if cw.last != cw.n { // total desconhecido: garante um último evento com o tamanho final
		onProgress(cw.n, cw.n)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// verifySHA256 compara hex sem distinguir maiúsculas; want == "" desliga a verificação.
func verifySHA256(got, want string) error {
	if want == "" {
		return nil
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("sha256 divergente: esperado %s, obtido %s", strings.ToLower(want), got)
	}
	return nil
}
