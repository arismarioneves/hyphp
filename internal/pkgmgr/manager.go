package pkgmgr

import (
	"archive/zip"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"hyphp/internal/runtime"
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
)

type Progress struct {
	PackageID string `json:"packageId"`
	Done      int64  `json:"done"`
	Total     int64  `json:"total"` // -1 se desconhecido
	Phase     string `json:"phase"` // uma das constantes Phase* acima
	Error     string `json:"error"`
}

type Manager struct {
	binDir string
	tmpDir string
	client *http.Client
}

// NewManager cria um Manager que instala em binDir usando tmpDir para downloads
// parciais. client nil usa http.DefaultClient (sem timeout global: o ctx de Install manda).
func NewManager(binDir, tmpDir string, client *http.Client) *Manager {
	if client == nil {
		client = http.DefaultClient
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
		onProgress(Progress{PackageID: pkg.ID, Phase: PhaseError, Error: err.Error()})
		return runtime.Installed{}, err
	}

	if err := os.MkdirAll(m.tmpDir, 0o755); err != nil {
		return fail(fmt.Errorf("pkgmgr: %w", err))
	}
	tmp := filepath.Join(m.tmpDir, pkg.ID+".part")
	defer os.Remove(tmp)

	report(PhaseDownload, 0, -1)
	sum, err := Download(ctx, m.client, pkg.URL, tmp, func(done, total int64) { report(PhaseDownload, done, total) })
	if err != nil {
		return fail(fmt.Errorf("pkgmgr: %w", err))
	}

	report(PhaseVerify, 0, 0)
	if err := verifySHA256(sum, pkg.SHA256); err != nil {
		return fail(fmt.Errorf("pkgmgr: %s: %w", pkg.ID, err))
	}

	report(PhaseExtract, 0, 0)
	destDir, err := m.place(pkg, tmp)
	if err != nil {
		return fail(fmt.Errorf("pkgmgr: %w", err))
	}

	inst, err := runtime.Detect(pkg.Kind, destDir)
	if err != nil {
		return fail(fmt.Errorf("pkgmgr: instalado em %s mas não detectado: %w", destDir, err))
	}
	report(PhaseDone, 0, 0)
	return inst, nil
}

// place coloca o arquivo baixado em bin/ e devolve a pasta que runtime.Detect deve ler.
// Regras (índice C5 + adição A4): .exe solto → bin/<kind>/<kind>.exe; Mailpit/Mkcert →
// direto em bin/<kind>/; demais → bin/<kind>/<raiz-do-zip>/ se a raiz única contiver a
// versão, senão bin/<kind>/<pkg.ID>/.
func (m *Manager) place(pkg Package, tmp string) (string, error) {
	parent := filepath.Join(m.binDir, string(pkg.Kind))
	if strings.HasSuffix(strings.ToLower(pkg.URL), ".exe") {
		if err := copyFile(tmp, filepath.Join(parent, string(pkg.Kind)+".exe")); err != nil {
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
