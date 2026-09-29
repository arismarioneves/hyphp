package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/wailsapp/wails/v3/pkg/application"

	"hyphp/internal/pkgmgr"
	"hyphp/internal/runtime"
	"hyphp/internal/state"
)

// binWatchDebounce agrupa a rajada de eventos de uma cópia/extração numa única varredura.
const binWatchDebounce = time.Second

type RuntimesDeps struct {
	App       *application.App // diálogo nativo de pasta; nil em smoke/testes
	BinDir    string
	TmpDir    string
	Manager   *pkgmgr.Manager
	Catalog   pkgmgr.Catalog
	State     *state.State
	StatePath string
	Logger    *slog.Logger
	Emit      func(name string, data any)    // app.Event.Emit; nil em smoke/testes
	OnChange  func(list []runtime.Installed) // opcional; chamado fora do lock após cada mudança
	// StopUsing para os serviços que rodam de dir (stack.StopUsingDir). Nil
	// em smoke/testes sem stack.
	StopUsing func(dir string) error
}

type RuntimesService struct {
	d          RuntimesDeps
	mu         sync.Mutex
	installed  []runtime.Installed
	scanned    bool
	installing map[string]context.CancelFunc // packageID → cancela o download em andamento
	ctx        context.Context // de ServiceStartup; Background() antes disso
}

func NewRuntimesService(d RuntimesDeps) *RuntimesService {
	d.BinDir = filepath.Clean(d.BinDir)
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &RuntimesService{d: d, installing: map[string]context.CancelFunc{}, ctx: context.Background()}
}

// ServiceStartup (Wails) inicia o watcher de bin/. ctx é cancelado no shutdown.
func (r *RuntimesService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	r.ctx = ctx
	go r.watchBin(ctx)
	return nil
}

// Installed devolve o cache da última varredura (varre na primeira chamada).
func (r *RuntimesService) Installed() []runtime.Installed {
	r.mu.Lock()
	scanned := r.scanned
	r.mu.Unlock()
	if !scanned {
		if err := r.Rescan(); err != nil {
			r.d.Logger.Warn("runtimes: falhas na varredura inicial", "err", err)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.installed)
}

// Rescan varre bin/, atualiza o cache e notifica (runtime:changed + OnChange).
// Falhas parciais de detecção voltam no erro, mas o cache é atualizado mesmo assim.
func (r *RuntimesService) Rescan() error {
	list, err := runtime.Scan(r.d.BinDir)
	r.mu.Lock()
	r.installed = list
	r.scanned = true
	r.mu.Unlock()
	r.notify(list)
	if err != nil {
		return fmt.Errorf("runtimes: %w", err)
	}
	return nil
}

// Available devolve os pacotes do catálogo cuja (Kind, Version) não está instalada.
func (r *RuntimesService) Available() []pkgmgr.Package {
	installed := r.Installed()
	have := make(map[string]bool, len(installed))
	for _, i := range installed {
		have[string(i.Kind)+"@"+i.Version] = true
	}
	out := make([]pkgmgr.Package, 0, len(r.d.Catalog.Packages))
	for _, p := range r.d.Catalog.Packages {
		if !have[string(p.Kind)+"@"+p.Version] {
			out = append(out, p)
		}
	}
	return out
}

// Install dispara o download em goroutine. Progresso vai por download:progress;
// ao terminar (com ou sem erro) faz Rescan, que emite runtime:changed.
func (r *RuntimesService) Install(packageID string) error {
	pkg, ok := r.d.Catalog.ByID(packageID)
	if !ok {
		return fmt.Errorf("pacote %q não existe no catálogo", packageID)
	}
	r.mu.Lock()
	if _, busy := r.installing[packageID]; busy {
		r.mu.Unlock()
		return fmt.Errorf("pacote %q já está sendo instalado", packageID)
	}
	ctx, cancel := context.WithCancel(r.ctx)
	r.installing[packageID] = cancel
	r.mu.Unlock()

	go func() {
		defer func() {
			r.mu.Lock()
			delete(r.installing, packageID)
			r.mu.Unlock()
			cancel()
		}()
		_, err := r.d.Manager.Install(ctx, pkg, func(p pkgmgr.Progress) { r.emit("download:progress", p) })
		switch {
		case errors.Is(err, context.Canceled):
			r.d.Logger.Info("runtimes: download cancelado", "pkg", packageID)
		case err != nil:
			r.d.Logger.Error("runtimes: instalação falhou", "pkg", packageID, "err", err)
		}
		if err := r.Rescan(); err != nil {
			r.d.Logger.Warn("runtimes: varredura após instalação", "err", err)
		}
	}()
	return nil
}

// CancelInstall interrompe o download em andamento do pacote. O progresso
// termina com a fase "canceled" e o arquivo parcial é apagado. Depois do
// download (verificação e extração, que levam segundos) não há o que cancelar.
func (r *RuntimesService) CancelInstall(packageID string) error {
	r.mu.Lock()
	cancel, ok := r.installing[packageID]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("pacote %q não está sendo instalado", packageID)
	}
	cancel()
	return nil
}

// Remove para os serviços que rodam do runtime, apaga a pasta e revarre. A
// revarredura dispara o Reconcile, que passa o serviço para outra versão
// instalada ou o tira.
func (r *RuntimesService) Remove(kind, version string) error {
	inst, ok := r.find(runtime.Kind(kind), version)
	if !ok {
		return fmt.Errorf("runtime %s %s não está instalado", kind, version)
	}
	if r.d.StopUsing != nil {
		if err := r.d.StopUsing(inst.Dir); err != nil {
			return err
		}
	}
	if err := r.d.Manager.Remove(inst); err != nil {
		return err
	}
	return r.Rescan()
}

// kindsImportaveis são os tipos que a importação aceita. Mailpit e mkcert ficam
// direto em bin/<kind>/, sem pasta por versão, e pesam poucos megabytes no
// catálogo: importá-los exigiria tratar um segundo layout de destino sem poupar
// download nenhum.
var kindsImportaveis = []runtime.Kind{runtime.PHP, runtime.Apache, runtime.Nginx, runtime.MySQL}

// copyRuntimeTree copia recursivamente src para dst. Copia, não move: a pasta
// de origem costuma ser de outra ferramenta que o usuário ainda usa, e o HyPHP
// não pode depender de uma instalação que não controla.
func copyRuntimeTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		alvo := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(alvo, 0o755)
		}
		// Links e dispositivos não interessam num runtime e só trariam
		// surpresa (um symlink para fora da árvore, por exemplo).
		if !d.Type().IsRegular() {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(alvo), 0o755); err != nil {
			return err
		}
		out, err := os.Create(alvo)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return out.Close()
	})
}

// destinoImport devolve a pasta de bin/ que vai receber inst.
func destinoImport(binDir string, inst runtime.Installed) string {
	nome := filepath.Base(inst.Dir)
	if strings.EqualFold(nome, string(inst.Kind)) {
		// O XAMPP guarda a build em <raiz>\php; copiada com o nome de origem
		// daria bin/php/php, que esconde a versão na lista e colidiria com a
		// próxima importação do mesmo tipo.
		nome = string(inst.Kind) + "-" + inst.Version
	}
	return filepath.Join(binDir, string(inst.Kind), nome)
}

// runtimesEm reconhece as builds sob dir. Cobre os três jeitos de o usuário
// apontar a pasta: uma raiz no layout do HyPHP/Laragon (<dir>/php/<build>), a
// pasta que contém as builds direto (<dir>/<build>, o bin\php do Laragon) e a
// própria build (ou a raiz do XAMPP, onde <dir>/php já é a build).
func runtimesEm(dir string) []runtime.Installed {
	var achadas []runtime.Installed
	vistas := map[string]bool{}
	add := func(inst runtime.Installed) {
		// Um bin/ no layout do HyPHP também traz mailpit e mkcert, que moram
		// direto em bin/<kind>/: copiados para bin/<kind>/<pasta> virariam
		// bytes que nenhuma varredura encontra.
		if !slices.Contains(kindsImportaveis, inst.Kind) || vistas[strings.ToLower(inst.Dir)] {
			return
		}
		vistas[strings.ToLower(inst.Dir)] = true
		achadas = append(achadas, inst)
	}

	// O erro do Scan só descreve builds que não rodam; elas simplesmente não
	// são importáveis, e falhar aqui esconderia as que deram certo.
	list, _ := runtime.Scan(dir)
	for _, inst := range list {
		add(inst)
	}

	candidatos := []string{dir}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				candidatos = append(candidatos, filepath.Join(dir, e.Name()))
			}
		}
	}
	for _, c := range candidatos {
		for _, kind := range kindsImportaveis {
			// Detect falha barato (os.Stat do executável) quando o candidato
			// não é daquele tipo.
			if inst, err := runtime.Detect(kind, c); err == nil {
				add(inst)
			}
		}
	}
	return achadas
}

// PickImportDir abre o diálogo nativo de pasta. Cancelar devolve ("", nil) —
// mesma comparação de mensagem de ProjectsService.PickRoot, pelo mesmo motivo
// (cfd é interno ao Wails, não há sentinela importável).
func (r *RuntimesService) PickImportDir() (string, error) {
	dir, err := r.d.App.Dialog.OpenFile().
		SetTitle("Selecionar pasta com runtimes (ex.: bin de outra ferramenta)").
		CanChooseDirectories(true).
		CanChooseFiles(false).
		PromptForSingleSelection()
	if err != nil {
		if strings.Contains(err.Error(), dialogCancelledMsg) {
			return "", nil
		}
		return "", fmt.Errorf("diálogo de pasta: %w", err)
	}
	return dir, nil
}

// ImportFrom copia para bin/ toda build reconhecível sob dir. Quem já tem PHP,
// Apache ou MySQL na máquina não precisa rebaixar centenas de megabytes.
func (r *RuntimesService) ImportFrom(dir string) error {
	achadas := runtimesEm(dir)
	if len(achadas) == 0 {
		return fmt.Errorf("nenhum runtime reconhecido em %s", dir)
	}
	for _, inst := range achadas {
		destino := destinoImport(r.d.BinDir, inst)
		if _, err := os.Stat(destino); err == nil {
			continue // já instalado: importar de novo só duplicaria bytes
		}
		if err := copyRuntimeTree(inst.Dir, destino); err != nil {
			// Uma árvore pela metade seria detectada como runtime quebrado na
			// próxima varredura; melhor não deixar rastro da cópia falha.
			os.RemoveAll(destino)
			return fmt.Errorf("copiar %s: %w", inst.Dir, err)
		}
	}
	// O watcher de bin/ publica a lista nova; forçar o rescan evita depender
	// do tempo de propagação do evento de arquivo.
	if err := r.Rescan(); err != nil {
		return fmt.Errorf("reler bin/: %w", err)
	}
	return nil
}

// SetDefaultPHP grava state.DefaultPHP e notifica (settings:changed + runtime:changed).
func (r *RuntimesService) SetDefaultPHP(major string) error {
	if _, ok := runtime.PHPByMajor(r.Installed(), major); !ok {
		return fmt.Errorf("PHP %s não está instalado", major)
	}
	r.d.State.DefaultPHP = major
	if err := state.Save(r.d.StatePath, *r.d.State); err != nil {
		return fmt.Errorf("salvar state: %w", err)
	}
	r.emit("settings:changed", *r.d.State)
	r.notify(r.Installed())
	return nil
}

// Extensions lista ext/*.dll do maior patch da série, marcando as habilitadas.
func (r *RuntimesService) Extensions(major string) ([]runtime.Extension, error) {
	inst, ok := runtime.PHPByMajor(r.Installed(), major)
	if !ok {
		return nil, fmt.Errorf("PHP %s não está instalado", major)
	}
	return runtime.ListExtensions(inst, r.enabledExtensions(major))
}

// SetExtension liga/desliga uma extensão da série e persiste em state.PHPExtensions.
func (r *RuntimesService) SetExtension(major, name string, on bool) error {
	inst, ok := runtime.PHPByMajor(r.Installed(), major)
	if !ok {
		return fmt.Errorf("PHP %s não está instalado", major)
	}
	available, err := runtime.ListExtensions(inst, nil)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(available, func(e runtime.Extension) bool { return e.Name == name }) {
		return fmt.Errorf("extensão %q não existe em %s", name, filepath.Join(inst.Dir, "ext"))
	}

	cur := slices.Clone(r.enabledExtensions(major))
	idx := slices.Index(cur, name)
	switch {
	case on && idx < 0:
		cur = append(cur, name)
		sort.Strings(cur)
	case !on && idx >= 0:
		cur = slices.Delete(cur, idx, idx+1)
	}
	if r.d.State.PHPExtensions == nil {
		r.d.State.PHPExtensions = map[string][]string{}
	}
	r.d.State.PHPExtensions[major] = cur
	if err := state.Save(r.d.StatePath, *r.d.State); err != nil {
		return fmt.Errorf("salvar state: %w", err)
	}
	r.emit("settings:changed", *r.d.State)
	r.notify(r.Installed())
	return nil
}

// enabledExtensions devolve a lista do state para a série, ou os defaults.
func (r *RuntimesService) enabledExtensions(major string) []string {
	if list, ok := r.d.State.PHPExtensions[major]; ok {
		return list
	}
	return runtime.DefaultExtensions
}

func (r *RuntimesService) find(kind runtime.Kind, version string) (runtime.Installed, bool) {
	for _, i := range r.Installed() {
		if i.Kind == kind && i.Version == version {
			return i, true
		}
	}
	return runtime.Installed{}, false
}

func (r *RuntimesService) emit(name string, data any) {
	if r.d.Emit != nil {
		r.d.Emit(name, data)
	}
}

// notify emite runtime:changed e chama OnChange. Nunca chamar com r.mu preso (C17).
func (r *RuntimesService) notify(list []runtime.Installed) {
	r.emit("runtime:changed", list)
	if r.d.OnChange != nil {
		r.d.OnChange(list)
	}
}

// watchBin observa bin/ e bin/<kind>/ (fsnotify não é recursivo) e, 1 s após o último
// evento, revarre. É o que faz "jogar uma pasta em bin/php" aparecer na UI sem reiniciar.
func (r *RuntimesService) watchBin(ctx context.Context) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		r.d.Logger.Warn("runtimes: watcher indisponível", "err", err)
		return
	}
	defer w.Close()

	addWatches := func() {
		if err := os.MkdirAll(r.d.BinDir, 0o755); err != nil {
			r.d.Logger.Warn("runtimes: criar bin/", "err", err)
			return
		}
		_ = w.Add(r.d.BinDir)
		entries, err := os.ReadDir(r.d.BinDir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				_ = w.Add(filepath.Join(r.d.BinDir, e.Name())) // Add é idempotente
			}
		}
	}
	addWatches()

	var timer *time.Timer
	var fire <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if ev.Has(fsnotify.Create) && filepath.Dir(ev.Name) == r.d.BinDir {
				addWatches() // bin/<kind> novo (ex.: primeira instalação de mailpit)
			}
			if timer == nil {
				timer = time.NewTimer(binWatchDebounce)
				fire = timer.C
			} else {
				timer.Reset(binWatchDebounce)
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			r.d.Logger.Warn("runtimes: watcher", "err", err)
		case <-fire:
			timer, fire = nil, nil
			if err := r.Rescan(); err != nil {
				r.d.Logger.Warn("runtimes: varredura após mudança em bin/", "err", err)
			}
		}
	}
}
