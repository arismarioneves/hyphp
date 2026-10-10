package pkgmgr

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hyphp/internal/runtime"
	"hyphp/internal/sysproc"
)

// Fases de Progress.Phase. A UI trata cada uma como união literal
// (ProgressPhase em frontend/src/lib/types.ts); emitir uma fase fora desta
// lista faz a tela Runtimes cair no default sem avisar.
const (
	PhaseDownload = "download"
	PhaseVerify   = "verify"
	PhaseExtract  = "extract"
	PhaseDone     = "done"
	PhaseError    = "error"
	// PhaseCanceled: o usuário cancelou. Fase própria para a UI voltar ao
	// estado inicial em vez de mostrar "context canceled" como erro.
	PhaseCanceled = "canceled"
)

type Progress struct {
	PackageID string `json:"packageId"`
	Done      int64  `json:"done"`
	Total     int64  `json:"total"` // -1 se desconhecido
	Phase     string `json:"phase"` // uma das constantes Phase* acima
	Error     string `json:"error"`
	// Message é a linha de saída do `brew install` no macOS, para a UI mostrar
	// o que está acontecendo (o brew não informa bytes). Vazio nos downloads
	// do catálogo.
	Message string `json:"message"`
}

type Manager struct {
	binDir string
	tmpDir string
	client *http.Client
}

// NewManager cria um Manager que instala em binDir usando tmpDir para downloads
// parciais. client nil usa um client com timeouts de transporte, que cobrem
// conexão e cabeçalhos; o corpo que para de chegar sem a conexão fechar é
// coberto pelo stallTimeout de Download. Não há Timeout total porque downloads
// grandes demoram; o ctx de Install manda.
func NewManager(binDir, tmpDir string, client *http.Client) *Manager {
	if client == nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.ResponseHeaderTimeout = 30 * time.Second
		tr.IdleConnTimeout = 90 * time.Second
		client = &http.Client{Transport: tr}
	}
	return &Manager{binDir: binDir, tmpDir: tmpDir, client: client}
}

// Install baixa, verifica, extrai e detecta pkg. O arquivo temporário é sempre
// removido. Em erro, onProgress recebe um último Progress com Phase "error".
func (m *Manager) Install(ctx context.Context, pkg Package, onProgress func(Progress)) (runtime.Installed, error) {
	if onProgress == nil {
		onProgress = func(Progress) {}
	}
	report := func(phase string, done, total int64) {
		onProgress(Progress{PackageID: pkg.ID, Done: done, Total: total, Phase: phase})
	}
	fail := func(err error) (runtime.Installed, error) {
		if errors.Is(err, context.Canceled) {
			onProgress(Progress{PackageID: pkg.ID, Phase: PhaseCanceled})
			return runtime.Installed{}, err
		}
		onProgress(Progress{PackageID: pkg.ID, Phase: PhaseError, Error: err.Error()})
		return runtime.Installed{}, err
	}

	if err := os.MkdirAll(m.tmpDir, 0o755); err != nil {
		return fail(fmt.Errorf("pkgmgr: %w", err))
	}
	tmp := filepath.Join(m.tmpDir, pkg.ID+".part")
	defer os.Remove(tmp)

	if err := m.fetch(ctx, pkg, tmp, report); err != nil {
		return fail(fmt.Errorf("pkgmgr: %s: %w", pkg.ID, err))
	}

	report(PhaseExtract, 0, 0)
	destDir, err := m.place(pkg, tmp)
	if err != nil {
		return fail(fmt.Errorf("pkgmgr: %w", err))
	}

	inst, err := runtime.Detect(pkg.Kind, destDir)
	if err != nil {
		// Pasta não detectável fica fora de Scan e do Remover da UI e bloqueia a
		// reinstalação; mesma regra de place: a pasta do kind é compartilhada.
		if destDir != filepath.Join(m.binDir, string(pkg.Kind)) {
			_ = os.RemoveAll(destDir)
		}
		return fail(fmt.Errorf("pkgmgr: instalado em %s mas não detectado: %w", destDir, err))
	}
	report(PhaseDone, 0, 0)
	return inst, nil
}

// fetch baixa pkg para tmp e confere o sha256, tentando a URL principal e
// depois cada mirror. Qualquer falha de uma fonte (HTML no lugar do arquivo,
// HTTP de erro, sha256 diferente, conexão interrompida) passa para a próxima;
// cancelar encerra na hora.
func (m *Manager) fetch(ctx context.Context, pkg Package, tmp string, report func(phase string, done, total int64)) error {
	var errs []error
	for _, url := range pkg.sources() {
		report(PhaseDownload, 0, -1)
		sum, err := Download(ctx, m.client, url, tmp, func(done, total int64) { report(PhaseDownload, done, total) })
		if err == nil {
			report(PhaseVerify, 0, 0)
			if err = verifySHA256(sum, pkg.SHA256); err == nil {
				return nil
			}
			err = fmt.Errorf("%s: %w", url, err)
		}
		if ctx.Err() != nil {
			return err
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Fetch baixa pkg para dest, conferido pelo sha256 e sem extrair. É o caminho
// dos pacotes que não são runtime (KindCACert): dest só aparece inteiro e
// conferido.
func (m *Manager) Fetch(ctx context.Context, pkg Package, dest string) error {
	if err := os.MkdirAll(m.tmpDir, 0o755); err != nil {
		return fmt.Errorf("pkgmgr: %w", err)
	}
	tmp := filepath.Join(m.tmpDir, pkg.ID+".part")
	defer os.Remove(tmp)
	if err := m.fetch(ctx, pkg, tmp, func(string, int64, int64) {}); err != nil {
		return fmt.Errorf("pkgmgr: %s: %w", pkg.ID, err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("pkgmgr: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return fmt.Errorf("pkgmgr: %w", err)
	}
	return nil
}

// place coloca o arquivo baixado em bin/ e devolve a pasta que runtime.Detect deve ler.
// Regras (índice C5 + adição A4): .exe solto → bin/<kind>/<kind>.exe; Mailpit/Mkcert →
// direto em bin/<kind>/; demais → bin/<kind>/<raiz-do-zip>/ se a raiz única contiver a
// versão, senão bin/<kind>/<pkg.ID>/.
func (m *Manager) place(pkg Package, tmp string) (string, error) {
	parent := filepath.Join(m.binDir, string(pkg.Kind))
	if strings.HasSuffix(strings.ToLower(pkg.URL), ".exe") {
		if err := copyFile(tmp, filepath.Join(parent, sysproc.ExeName(string(pkg.Kind)))); err != nil {
			return "", err
		}
		return parent, nil
	}

	zr, err := zip.OpenReader(tmp)
	if err != nil {
		return "", fmt.Errorf("abrir zip de %s: %w", pkg.ID, err)
	}
	defer zr.Close()

	root := zipRoot(&zr.Reader)
	destDir := parent
	if pkg.Kind != runtime.Mailpit && pkg.Kind != runtime.Mkcert {
		if root != "" && strings.Contains(root, pkg.Version) {
			destDir = filepath.Join(parent, root)
		} else {
			destDir = filepath.Join(parent, pkg.ID)
		}
		if _, err := os.Stat(destDir); err == nil {
			return "", fmt.Errorf("%s já existe; remova antes de reinstalar", destDir)
		}
	}
	if err := extractZip(&zr.Reader, destDir, root); err != nil {
		if destDir != parent {
			_ = os.RemoveAll(destDir) // não deixar instalação pela metade
		}
		return "", err
	}
	return destDir, nil
}

// Remove apaga a pasta do runtime. Falha se algum processo (php-cgi, mysqld) ainda
// segurar arquivos — o chamador para o serviço antes (RuntimesDeps.StopUsing).
func (m *Manager) Remove(inst runtime.Installed) error {
	if inst.Dir == "" || inst.Dir == m.binDir {
		return fmt.Errorf("pkgmgr: recusando remover %q", inst.Dir)
	}
	if err := os.RemoveAll(inst.Dir); err != nil {
		return fmt.Errorf("pkgmgr: remover %s: %w", inst.Dir, err)
	}
	return nil
}
