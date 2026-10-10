package services

import (
	"context"
	"errors"
	"fmt"
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

	"hyphp/internal/i18n"
	"hyphp/internal/pkgmgr"
	"hyphp/internal/render"
	"hyphp/internal/runtime"
	"hyphp/internal/state"
)

// binWatchDebounce agrupa a rajada de eventos de uma cópia/extração numa única varredura.
const binWatchDebounce = time.Second

type RuntimesDeps struct {
	App     *application.App // diálogo nativo de pasta; nil em smoke/testes
	BinDir  string
	TmpDir  string
	Manager *pkgmgr.Manager
	// Catalog devolve o catálogo em uso: o embutido ou o remoto mais novo já
	// aceito. É função, e não valor, porque o catálogo remoto troca sem
	// reiniciar o app.
	Catalog func() pkgmgr.Catalog
	// UpdateState aplica a mutação sob o lock do Stack e persiste; State
	// devolve uma cópia. O state é compartilhado com o Reconcile, que o
	// serializa em outra goroutine: mexer nos mapas sem o lock derruba o
	// processo (concurrent map read and map write).
	UpdateState func(func(*state.State)) error
	State       func() state.State
	Logger      *slog.Logger
	Emit        func(name string, data any)    // app.Event.Emit; nil em smoke/testes
	OnChange    func(list []runtime.Installed) // opcional; chamado fora do lock após cada mudança
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
	ctx        context.Context               // de ServiceStartup; Background() antes disso

	// Consultas ao php.exe da série (runtime.PHPIniBuiltins/PHPIniKnows e
	// runtime.BuiltinModules). São campos para que os testes das regras de
	// php.ini e de extensões não dependam de um PHP instalado na máquina.
	iniBuiltins    func(ctx context.Context, inst runtime.Installed, ext, names []string) (map[string]string, error)
	iniKnows       func(ctx context.Context, inst runtime.Installed, ext []string, name, value string) (bool, error)
	builtinModules func(ctx context.Context, inst runtime.Installed) ([]string, error)

	src sourceState // estado da fonte de runtimes por SO (Homebrew no Mac); sob mu
	// watchRefresh avisa o watcher que as pastas extras mudaram (Homebrew
	// achado ou prefixo trocado). Buffer 1 com envio não bloqueante: vários
	// avisos seguidos viram uma releitura só. No Windows nunca recebe nada.
	watchRefresh chan struct{}
}

func NewRuntimesService(d RuntimesDeps) *RuntimesService {
	d.BinDir = filepath.Clean(d.BinDir)
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &RuntimesService{
		d: d, installing: map[string]context.CancelFunc{}, ctx: context.Background(),
		iniBuiltins: runtime.PHPIniBuiltins, iniKnows: runtime.PHPIniKnows, builtinModules: runtime.BuiltinModules,
		src: newSourceState(), watchRefresh: make(chan struct{}, 1),
	}
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

// Rescan varre as fontes do SO, atualiza o cache e notifica (runtime:changed + OnChange).
// Falhas parciais de detecção voltam no erro, mas o cache é atualizado mesmo assim.
func (r *RuntimesService) Rescan() error {
	list, err := r.scan()
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

// Available devolve o que pode ser instalado e ainda não está (regra por SO).
func (r *RuntimesService) Available() []pkgmgr.Package {
	return r.available(r.Installed())
}

// BrewStatus diz à UI se a plataforma usa Homebrew e se ele foi encontrado.
// Supported=false no Windows: é também o sinal de plataforma do frontend.
type BrewStatus struct {
	Supported      bool   `json:"supported"`
	Found          bool   `json:"found"`
	Prefix         string `json:"prefix"`
	InstallCommand string `json:"installCommand"`
}

// installFunc instala um pacote já resolvido, emitindo o progresso em on.
type installFunc func(ctx context.Context, on func(pkgmgr.Progress)) error

// Install dispara a instalação em goroutine. Progresso vai por download:progress;
// ao terminar (com ou sem erro) faz Rescan, que emite runtime:changed.
func (r *RuntimesService) Install(packageID string) error {
	install, err := r.installer(packageID)
	if err != nil {
		return err
	}
	r.mu.Lock()
	if _, busy := r.installing[packageID]; busy {
		r.mu.Unlock()
		return i18n.Errorf("err.runtimes.installing", packageID)
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
		err := install(ctx, func(p pkgmgr.Progress) { r.emit("download:progress", p) })
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
		return i18n.Errorf("err.runtimes.notInstalling", packageID)
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
		return i18n.Errorf("err.runtimes.notInstalled", kind, version)
	}
	if r.d.StopUsing != nil {
		if err := r.d.StopUsing(inst.Dir); err != nil {
			return err
		}
	}
	if err := r.remove(inst); err != nil {
		return err
	}
	return r.Rescan()
}

// SetDefaultPHP grava state.DefaultPHP e notifica (settings:changed + runtime:changed).
func (r *RuntimesService) SetDefaultPHP(major string) error {
	if _, ok := runtime.PHPByMajor(r.Installed(), major); !ok {
		return i18n.Errorf("err.runtimes.phpMissing", major)
	}
	if err := r.d.UpdateState(func(st *state.State) { st.DefaultPHP = major }); err != nil {
		return i18n.Errorf("err.runtimes.saveState", err)
	}
	r.emit("settings:changed", r.d.State())
	r.notify(r.Installed())
	return nil
}

// Extensions lista os módulos do maior patch da série: os embutidos no binário
// e os arquivos de ExtDir, marcando os habilitados.
func (r *RuntimesService) Extensions(major string) ([]runtime.Extension, error) {
	inst, ok := runtime.PHPByMajor(r.Installed(), major)
	if !ok {
		return nil, i18n.Errorf("err.runtimes.phpMissing", major)
	}
	return r.listExtensions(inst, r.enabledExtensions(major))
}

// listExtensions pergunta ao PHP o que vem compilado e junta com ExtDir.
func (r *RuntimesService) listExtensions(inst runtime.Installed, enabled []string) ([]runtime.Extension, error) {
	ctx, cancel := context.WithTimeout(r.ctx, iniProbeTimeout)
	defer cancel()
	builtin, err := r.builtinModules(ctx, inst)
	if err != nil {
		return nil, i18n.Errorf("err.runtimes.iniProbe", inst.Major, err)
	}
	return runtime.ListExtensions(inst, enabled, builtin)
}

// SetExtension liga/desliga uma extensão da série e persiste em state.PHPExtensions.
// Embutida é recusada: está sempre ligada, e gravar o nome no state não muda
// nada (o render só escreve extension= para módulo com arquivo).
func (r *RuntimesService) SetExtension(major, name string, on bool) error {
	inst, ok := runtime.PHPByMajor(r.Installed(), major)
	if !ok {
		return i18n.Errorf("err.runtimes.phpMissing", major)
	}
	available, err := r.listExtensions(inst, nil)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(available, func(e runtime.Extension) bool { return e.Name == name })
	if i < 0 {
		return i18n.Errorf("err.runtimes.unknownExtension", name, inst.ExtDir)
	}
	if available[i].Builtin {
		return i18n.Errorf("err.runtimes.builtinExtension", name, major)
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
	if err := r.d.UpdateState(func(st *state.State) {
		if st.PHPExtensions == nil {
			st.PHPExtensions = map[string][]string{}
		}
		st.PHPExtensions[major] = cur
	}); err != nil {
		return i18n.Errorf("err.runtimes.saveState", err)
	}
	r.emit("settings:changed", r.d.State())
	r.notify(r.Installed())
	return nil
}

// enabledExtensions devolve a lista do state para a série, ou os defaults.
func (r *RuntimesService) enabledExtensions(major string) []string {
	if list, ok := r.d.State().PHPExtensions[major]; ok {
		return list
	}
	return runtime.DefaultExtensions
}

// IniSetting é uma linha do painel de php.ini: o valor efetivo da diretiva na
// série e de onde ele vem.
type IniSetting struct {
	Name         string `json:"name"`
	Value        string `json:"value"`        // o que vai valer nos workers
	DefaultValue string `json:"defaultValue"` // o que vale sem a escolha do usuário
	Source       string `json:"source"`       // "user" | "hyphp" | "php"
}

// curatedIni são as diretivas que o painel mostra sempre: as que projetos
// comuns (Moodle, Magento, WordPress com importação grande) pedem para mudar.
var curatedIni = []string{
	"memory_limit", "max_execution_time", "max_input_time", "max_input_vars",
	"post_max_size", "upload_max_filesize", "display_errors", "error_reporting",
	"date.timezone", "opcache.enable",
}

// iniProbeTimeout limita cada consulta ao php.exe; o PHP responde em
// milissegundos, mas um antivírus inspecionando o exe não pode travar a UI.
const iniProbeTimeout = 15 * time.Second

// IniSettings lista as diretivas curadas e as que o usuário definiu para a
// série, com valor efetivo, padrão e origem. Curadas primeiro, na ordem da
// lista; depois as extras por nome.
func (r *RuntimesService) IniSettings(major string) ([]IniSetting, error) {
	inst, ok := runtime.PHPByMajor(r.Installed(), major)
	if !ok {
		return nil, i18n.Errorf("err.runtimes.phpMissing", major)
	}
	user := r.d.State().PHPIni[major]
	names := slices.Clone(curatedIni)
	var extras []string
	for name := range user {
		// Gerenciadas (curl.cainfo/openssl.cafile no Windows) ficam de fora: o
		// RenderPHPIni as ignora, então um valor antigo salvo na 3.x apareceria
		// como "user" sem valer no php.ini.
		if !slices.Contains(names, name) && !render.IsManagedIniDirective(name) {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	names = append(names, extras...)

	ctx, cancel := context.WithTimeout(r.ctx, iniProbeTimeout)
	defer cancel()
	builtin, err := r.iniBuiltins(ctx, inst, r.enabledExtensions(major), slices.DeleteFunc(slices.Clone(names), func(n string) bool { return !runtime.ValidIniName(n) }))
	if err != nil {
		return nil, i18n.Errorf("err.runtimes.iniProbe", major, err)
	}
	hyphp := map[string]string{}
	for _, d := range render.PHPIniDefaults() {
		hyphp[d.Name] = d.Value
	}

	out := make([]IniSetting, 0, len(names))
	for _, name := range names {
		s := IniSetting{Name: name}
		if v, ok := hyphp[name]; ok {
			s.DefaultValue, s.Source = v, "hyphp"
		} else if v, ok := builtin[name]; ok {
			s.DefaultValue, s.Source = v, "php"
		} else if _, ok := user[name]; !ok {
			continue // curada que este PHP não conhece (ex.: extensão ausente)
		}
		s.Value = s.DefaultValue
		if v, ok := user[name]; ok {
			s.Value, s.Source = v, "user"
		}
		out = append(out, s)
	}
	return out, nil
}

// SetIniSetting define name=value no php.ini da série e persiste em
// state.PHPIni. O Reconcile que o notify dispara regrava o php.ini e reinicia
// só os workers dessa série.
func (r *RuntimesService) SetIniSetting(major, name, value string) error {
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if !runtime.ValidIniName(name) {
		return i18n.Errorf("err.runtimes.iniName", name)
	}
	if render.IsManagedIniDirective(name) {
		return i18n.Errorf("err.runtimes.iniManaged", name)
	}
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return i18n.Errorf("err.runtimes.iniValue", name)
	}
	inst, ok := runtime.PHPByMajor(r.Installed(), major)
	if !ok {
		return i18n.Errorf("err.runtimes.phpMissing", major)
	}
	ctx, cancel := context.WithTimeout(r.ctx, iniProbeTimeout)
	defer cancel()
	known, err := r.iniKnows(ctx, inst, r.enabledExtensions(major), name, value)
	if err != nil {
		return i18n.Errorf("err.runtimes.iniProbe", major, err)
	}
	if !known {
		return i18n.Errorf("err.runtimes.iniUnknown", major, name)
	}

	return r.saveIni(func(st *state.State) {
		if st.PHPIni == nil {
			st.PHPIni = map[string]map[string]string{}
		}
		if st.PHPIni[major] == nil {
			st.PHPIni[major] = map[string]string{}
		}
		st.PHPIni[major][name] = value
	})
}

// ResetIniSetting apaga a escolha do usuário; a diretiva volta ao padrão do
// HyPHP ou do PHP. Sem diretivas, a série sai do mapa para o state.json não
// acumular objetos vazios.
func (r *RuntimesService) ResetIniSetting(major, name string) error {
	if _, ok := r.d.State().PHPIni[major][name]; !ok {
		return nil
	}
	return r.saveIni(func(st *state.State) {
		delete(st.PHPIni[major], name)
		if len(st.PHPIni[major]) == 0 {
			delete(st.PHPIni, major)
		}
	})
}

// saveIni aplica fn em state.PHPIni e dispara o Reconcile, como SetExtension.
func (r *RuntimesService) saveIni(fn func(*state.State)) error {
	if err := r.d.UpdateState(fn); err != nil {
		return i18n.Errorf("err.runtimes.saveState", err)
	}
	r.emit("settings:changed", r.d.State())
	r.notify(r.Installed())
	return nil
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
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					_ = w.Add(filepath.Join(r.d.BinDir, e.Name())) // Add é idempotente
				}
			}
		}
		// No Mac, <prefix>/opt: kegs instalados ou atualizados pelo terminal
		// aparecem sem reiniciar o app.
		for _, dir := range r.extraWatchDirs() {
			_ = w.Add(dir)
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
		case <-r.watchRefresh:
			addWatches()
		case <-fire:
			timer, fire = nil, nil
			if err := r.Rescan(); err != nil {
				r.d.Logger.Warn("runtimes: varredura após mudança em bin/", "err", err)
			}
		}
	}
}
