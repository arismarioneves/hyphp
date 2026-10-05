package pkgmgr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// progressStep é o intervalo de bytes entre chamadas de onProgress.
const progressStep = 256 * 1024

// stallTimeout é quanto o download tolera sem receber nenhum byte. Os
// timeouts do transporte só cobrem conexão e cabeçalhos, e o do updater é de
// 30 min no total: uma conexão que para de entregar o corpo sem fechar (Wi-Fi
// que cai, proxy que segura) deixaria a barra de progresso congelada. É var
// para os testes encurtarem.
var stallTimeout = 60 * time.Second

// ErrStalled é a causa do download abortado por stallTimeout. Não embrulha
// context.Canceled de propósito: para o Install é falha, não cancelamento do
// usuário.
var ErrStalled = errors.New("o servidor parou de enviar dados")

// idleReader rearma o watchdog a cada leitura que traz bytes: o que expira é a
// falta de progresso, não a duração, porque runtimes grandes em conexão lenta
// demoram e estão vivos.
type idleReader struct {
	r     io.Reader
	timer resetter
	d     time.Duration
}

// resetter é o pedaço do *time.Timer que o idleReader usa; a interface deixa o
// teste conferir o rearme sem depender do relógio.
type resetter interface{ Reset(time.Duration) bool }

func (ir *idleReader) Read(p []byte) (int, error) {
	n, err := ir.r.Read(p)
	if n > 0 {
		ir.timer.Reset(ir.d)
	}
	return n, err
}

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

// Download baixa url para dst (criado/truncado) em streaming, reportando progresso
// (total = -1 quando o servidor não informa Content-Length) e devolvendo o SHA-256
// hex minúsculo do conteúdo. Respostas text/html são rejeitadas: Apache Lounge
// responde 200 + HTML "Oops" quando uma build foi substituída. Ficar
// stallTimeout sem receber nada aborta com ErrStalled.
func Download(ctx context.Context, client *http.Client, url, dst string, onProgress func(done, total int64)) (string, error) {
	// O watchdog já corre durante o client.Do: o client do updater não tem
	// ResponseHeaderTimeout, e um servidor que aceita e não responde travaria
	// do mesmo jeito.
	d := stallTimeout
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	idle := time.AfterFunc(d, func() { cancel(ErrStalled) })
	defer idle.Stop()
	// O transporte devolve só "context canceled" ao abortar; a causa diz se
	// foi o watchdog ou o ctx do chamador.
	wrap := func(err error) error {
		if errors.Is(context.Cause(ctx), ErrStalled) {
			return fmt.Errorf("download %s: %w há %s (conexão caiu?)", url, ErrStalled, d)
		}
		return fmt.Errorf("download %s: %w", url, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "hyphp/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", wrap(err)
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
	_, copyErr := io.Copy(io.MultiWriter(f, h, cw), &idleReader{r: resp.Body, timer: idle, d: d})
	closeErr := f.Close()
	if copyErr != nil {
		return "", wrap(copyErr)
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
