package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"hyphp/internal/i18n"
	"hyphp/internal/pkgmgr"
)

// GitHubRepo é onde as versões são publicadas, como releases. Entra na URL
// gravada em cada binário: trocar de dono ou de nome corta o update de todas
// as versões já instaladas.
const GitHubRepo = "arismarioneves/hyphp"

// ManifestURL aponta para o asset latest.json da release marcada como Latest:
// o GitHub redireciona latest/download/<arquivo> para ela. O instalador sai
// da mesma pasta, porque o path do manifesto é só o nome do asset.
const ManifestURL = "https://github.com/" + GitHubRepo + "/releases/latest/download/latest.json"

// EnvURL substitui ManifestURL; é por ela que o smoke aponta para um servidor
// local. A assinatura continua obrigatória, então a variável não abre brecha.
const EnvURL = "HYPHP_UPDATE_URL"

// State é a fase do updater, exibida pela UI.
type State string

const (
	StateInactive    State = "inativo" // build de desenvolvimento
	StateIdle        State = "ocioso"
	StateChecking    State = "verificando"
	StateUpToDate    State = "em-dia"
	StateDownloading State = "baixando"
	StateReady       State = "pronto"
	StateApplying    State = "aplicando"
	StateFailed      State = "falhou"
)

// Status é o que a UI recebe no evento update:status.
type Status struct {
	State     State    `json:"state"`
	Current   string   `json:"current"`
	Available string   `json:"available"`
	Notes     []string `json:"notes"`
	Done      int64    `json:"done"`
	Total     int64    `json:"total"`
	Error     string   `json:"error"`
	CheckedAt string   `json:"checkedAt"`
	Message   string   `json:"message"` // resultado do último update aplicado
}

// Config são as dependências do Updater.
type Config struct {
	Current   string
	URL       string // latest.json
	Dir       string // var/update
	Key       ed25519.PublicKey
	Client    *http.Client
	Enabled   bool        // BuildEnabled em produção
	AutoCheck func() bool // toggle de Configurações; lido a cada ciclo
	OnStatus  func(Status)
	Logger    *slog.Logger

	FirstDelay time.Duration // 0 → 60 s: não disputar rede e CPU com o boot da stack
	Interval   time.Duration // 0 → 6 h
}

// Updater verifica e baixa versões novas. Seguro para uso concorrente.
type Updater struct {
	c Config

	mu     sync.Mutex
	st     Status
	latest *Latest // último manifesto verificado com versão nova

	checking sync.Mutex // um Check por vez; o laço e o botão "Verificar agora" disputam
}

const (
	manifestLimit = 1 << 20
	sigLimit      = 4 << 10
	fetchTimeout  = 30 * time.Second
	resultFile    = "resultado.json"
	updaterExe    = "hyphp-updater.exe"
)

// DefaultURL devolve EnvURL ou ManifestURL. Fora do loopback exige HTTPS.
func DefaultURL() (string, error) {
	raw := os.Getenv(EnvURL)
	if raw == "" {
		return ManifestURL, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("update: %s inválida: %w", EnvURL, err)
	}
	switch {
	case u.Scheme == "https" && u.Host != "":
		return raw, nil
	case u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"):
		return raw, nil
	}
	return "", fmt.Errorf("update: %s precisa ser https (http só em 127.0.0.1/localhost): %q", EnvURL, raw)
}

// New cria o Updater. Não faz I/O: Start é quem lê o resultado anterior e
// agenda as verificações.
func New(c Config) *Updater {
	if c.FirstDelay == 0 {
		c.FirstDelay = 60 * time.Second
	}
	if c.Interval == 0 {
		c.Interval = 6 * time.Hour
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	st := Status{State: StateIdle, Current: c.Current}
	if !c.Enabled {
		st.State = StateInactive
	}
	return &Updater{c: c, st: st}
}

// Start lê o resultado do último update e agenda as verificações até ctx
// terminar. Em build de dev não faz nada.
func (u *Updater) Start(ctx context.Context) {
	if !u.c.Enabled {
		return
	}
	u.loadResult()
	go func() {
		t := time.NewTimer(u.c.FirstDelay)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			if u.c.AutoCheck == nil || u.c.AutoCheck() {
				if err := u.Check(ctx); err != nil {
					u.c.Logger.Warn("verificar atualização", "err", err)
				}
			}
			t.Reset(u.c.Interval)
		}
	}()
}

// Status devolve uma cópia do estado atual.
func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	s := u.st
	s.Notes = append([]string(nil), u.st.Notes...)
	return s
}

func (u *Updater) set(fn func(*Status)) {
	u.mu.Lock()
	fn(&u.st)
	s := u.st
	s.Notes = append([]string(nil), u.st.Notes...)
	u.mu.Unlock()
	if u.c.OnStatus != nil {
		u.c.OnStatus(s)
	}
}

func (u *Updater) fail(err error) error {
	u.set(func(s *Status) {
		s.State = StateFailed
		s.Error = err.Error()
		s.Done, s.Total = 0, 0
	})
	return err
}

// Check busca o manifesto e, havendo versão nova, baixa e confere o
// instalador. Uma verificação já em curso faz esta devolver nil sem repetir.
func (u *Updater) Check(ctx context.Context) error {
	if !u.c.Enabled {
		return errors.New("update: builds de desenvolvimento não se atualizam")
	}
	if !u.checking.TryLock() {
		return nil
	}
	defer u.checking.Unlock()
	if s := u.Status(); s.State == StateApplying {
		return nil
	}

	u.set(func(s *Status) { s.State, s.Error = StateChecking, "" })
	l, err := u.fetchLatest(ctx)
	if err != nil {
		return u.fail(err)
	}
	now := time.Now().Format(time.RFC3339)
	c, err := Compare(l.Version, u.c.Current)
	if err != nil {
		return u.fail(err)
	}
	if c <= 0 {
		u.clean("")
		u.mu.Lock()
		u.latest = nil
		u.mu.Unlock()
		u.set(func(s *Status) {
			s.State, s.CheckedAt, s.Available, s.Notes = StateUpToDate, now, "", nil
		})
		return nil
	}

	u.clean(l.Version)
	final := u.installerPath(l)
	u.set(func(s *Status) {
		s.CheckedAt, s.Available, s.Notes = now, l.Version, append([]string(nil), l.Notes...)
	})
	// Arquivo final só existe depois de verificado; a conferência de novo
	// cobre o caso de ele ter sido mexido entre um boot e outro.
	if verifyFile(final, l.WindowsAMD64) != nil {
		if err := u.download(ctx, l, final); err != nil {
			return u.fail(err)
		}
	}
	u.mu.Lock()
	u.latest = &l
	u.mu.Unlock()
	u.set(func(s *Status) { s.State, s.Done, s.Total = StateReady, 0, 0 })
	u.c.Logger.Info("atualização pronta", "versao", l.Version, "arquivo", final)
	return nil
}

func (u *Updater) fetchLatest(ctx context.Context) (Latest, error) {
	body, err := u.get(ctx, u.c.URL, manifestLimit)
	if err != nil {
		return Latest{}, err
	}
	sig, err := u.get(ctx, u.c.URL+".sig", sigLimit)
	if err != nil {
		return Latest{}, err
	}
	// Assinatura antes de interpretar qualquer campo: manifesto não assinado
	// não decide nem o que baixar.
	if err := Verify(u.c.Key, body, sig); err != nil {
		return Latest{}, err
	}
	return ParseLatest(body)
}

func (u *Updater) get(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "hyphp/"+u.c.Current)
	resp, err := u.c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: buscar %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: %s respondeu HTTP %d", rawURL, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("update: ler %s: %w", rawURL, err)
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("update: %s passa de %d bytes", rawURL, limit)
	}
	return raw, nil
}

func (u *Updater) download(ctx context.Context, l Latest, final string) error {
	a := l.WindowsAMD64
	base, err := url.Parse(u.c.URL)
	if err != nil {
		return err
	}
	src := base.ResolveReference(&url.URL{Path: a.Path}).String()
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return err
	}
	part := final + ".part"
	defer os.Remove(part) // no sucesso já foi renomeado; na falha não pode sobrar

	u.set(func(s *Status) { s.State, s.Done, s.Total = StateDownloading, 0, a.Size })
	sum, err := pkgmgr.Download(ctx, u.c.Client, src, part, func(done, _ int64) {
		u.set(func(s *Status) { s.Done = done })
	})
	if err != nil {
		return fmt.Errorf("update: baixar %s: %w", src, err)
	}
	if st, err := os.Stat(part); err != nil || st.Size() != a.Size {
		return fmt.Errorf("update: instalador com tamanho diferente do anunciado (%d bytes)", a.Size)
	}
	if sum != a.SHA256 {
		return fmt.Errorf("update: sha256 do instalador não confere (anunciado %s, obtido %s)", a.SHA256, sum)
	}
	return os.Rename(part, final)
}

func (u *Updater) installerPath(l Latest) string {
	return filepath.Join(u.c.Dir, l.Version, path.Base(l.WindowsAMD64.Path))
}

// clean apaga os instaladores de versões diferentes de keep ("" apaga todos).
func (u *Updater) clean(keep string) {
	entries, err := os.ReadDir(u.c.Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != keep {
			_ = os.RemoveAll(filepath.Join(u.c.Dir, e.Name()))
		}
	}
}

func verifyFile(p string, a *Artifact) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return err
	}
	if n != a.Size || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		return fmt.Errorf("update: %s não confere com o manifesto", p)
	}
	return nil
}

// loadResult lê o resultado.json deixado pelo atualizador e o remove: a
// mensagem aparece uma vez.
func (u *Updater) loadResult() {
	p := filepath.Join(u.c.Dir, resultFile)
	raw, err := os.ReadFile(p)
	if err != nil {
		return
	}
	_ = os.Remove(p)
	var r Result
	if err := json.Unmarshal(raw, &r); err != nil {
		u.c.Logger.Warn("resultado do update ilegível", "err", err)
		return
	}
	u.c.Logger.Info("resultado do último update", "de", r.From, "para", r.To, "ok", r.OK, "erro", r.Error)
	switch {
	case !r.OK:
		u.set(func(s *Status) { s.State, s.Error = StateFailed, r.Error })
	case r.To != u.c.Current:
		// Instalador que diz ter terminado bem mas deixa o binário antigo é o
		// começo de um loop de update.
		u.set(func(s *Status) {
			s.State = StateFailed
			s.Error = i18n.T("update.installerMismatch", r.To, u.c.Current)
		})
	default:
		u.clean("")
		u.set(func(s *Status) { s.Message = i18n.T("update.done", r.To) })
	}
}

// Prepare confere de novo o instalador pronto e monta o pedido para Launch.
// Passa o estado para "aplicando".
func (u *Updater) Prepare() (ApplyRequest, error) {
	u.mu.Lock()
	l, st := u.latest, u.st.State
	u.mu.Unlock()
	if st != StateReady || l == nil {
		return ApplyRequest{}, errors.New("update: nenhuma versão pronta para instalar")
	}
	inst := u.installerPath(*l)
	// var/update é gravável pelo usuário: o que foi verificado no download
	// pode não ser o que está no disco agora.
	if err := verifyFile(inst, l.WindowsAMD64); err != nil {
		return ApplyRequest{}, u.fail(err)
	}
	self, err := os.Executable()
	if err != nil {
		return ApplyRequest{}, err
	}
	u.set(func(s *Status) { s.State, s.Error = StateApplying, "" })
	return ApplyRequest{
		PID:       os.Getpid(),
		Installer: inst,
		SHA256:    l.WindowsAMD64.SHA256,
		Size:      l.WindowsAMD64.Size,
		Dir:       filepath.Dir(self),
		Exe:       filepath.Base(self),
		Result:    filepath.Join(u.c.Dir, resultFile),
		From:      u.c.Current,
		To:        l.Version,
	}, nil
}

// Apply prepara e inicia o atualizador. Só chama quit depois que o atualizador
// está de pé; se ele não subir, o app segue aberto e volta a "pronto".
func (u *Updater) Apply(quit func()) error {
	req, err := u.Prepare()
	if err != nil {
		return err
	}
	if err := Launch(req, filepath.Join(u.c.Dir, updaterExe)); err != nil {
		u.set(func(s *Status) { s.State, s.Error = StateReady, err.Error() })
		return err
	}
	u.c.Logger.Info("atualizador iniciado; encerrando o app", "de", req.From, "para", req.To)
	quit()
	return nil
}
