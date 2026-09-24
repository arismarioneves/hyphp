package stack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"hyphp/internal/elevate"
	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
	"hyphp/internal/project"
	"hyphp/internal/render"
	"hyphp/internal/runtime"
	"hyphp/internal/state"
	"hyphp/internal/supervisor"
	"hyphp/internal/webserver"
)

// Deps é o contrato C11.
type Deps struct {
	Sup       *supervisor.Supervisor
	State     *state.State
	StatePath string
	Runtimes  []runtime.Installed
	Projects  []project.Project
	Web       map[state.WebServerName]webserver.WebServer
	Alloc     *netcfg.Allocator
	Mkcert    netcfg.Mkcert
	Logger    *slog.Logger
	Emit      func(name string, data any)
}

// Stack aplica o estado desejado ao supervisor.
//
// Locks: mu serializa Reconcile/StartAll/StopAll/SwitchWebServer (operações
// longas, com I/O e UAC); stateMu protege *d.State para leituras rápidas dos
// services enquanto mu está preso. Nunca segurar stateMu ao chamar o supervisor.
type Stack struct {
	mu      sync.Mutex
	stateMu sync.RWMutex
	d       Deps

	applied  map[string]supervisor.Spec // specs hoje registrados no supervisor
	started  bool                       // StartAll já rodou → specs novos sobem no Reconcile
	warnings []Warning
	caReady  bool // mkcert -install já confirmado nesta sessão
}

func New(d Deps) *Stack {
	return &Stack{d: d, applied: map[string]supervisor.Spec{}}
}

// ---- estado compartilhado -------------------------------------------------

func (s *Stack) State() state.State {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return cloneState(*s.d.State)
}

// UpdateState aplica fn sob lock e persiste. Não reconcilia — quem chama decide.
func (s *Stack) UpdateState(fn func(*state.State)) error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	fn(s.d.State)
	if err := state.Save(s.d.StatePath, *s.d.State); err != nil {
		return fmt.Errorf("stack: salvar state: %w", err)
	}
	return nil
}

func (s *Stack) Projects() []project.Project {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return append([]project.Project(nil), s.d.Projects...)
}

func (s *Stack) SetProjects(p []project.Project) {
	s.stateMu.Lock()
	s.d.Projects = append([]project.Project(nil), p...)
	s.stateMu.Unlock()
}

func (s *Stack) SetRuntimes(r []runtime.Installed) {
	s.stateMu.Lock()
	s.d.Runtimes = append([]runtime.Installed(nil), r...)
	s.stateMu.Unlock()
}

func (s *Stack) Warnings() []Warning {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return append([]Warning(nil), s.warnings...)
}

func cloneState(st state.State) state.State {
	st.Roots = append([]string(nil), st.Roots...)
	if st.PortAlloc != nil {
		pa := make(map[string][]int, len(st.PortAlloc))
		for k, v := range st.PortAlloc {
			pa[k] = append([]int(nil), v...)
		}
		st.PortAlloc = pa
	}
	if st.PHPExtensions != nil {
		pe := make(map[string][]string, len(st.PHPExtensions))
		for k, v := range st.PHPExtensions {
			pe[k] = append([]string(nil), v...)
		}
		st.PHPExtensions = pe
	}
	return st
}

// snapshot copia o que desired precisa, sem segurar stateMu durante I/O.
func (s *Stack) snapshot() (state.State, []runtime.Installed, []project.Project) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return cloneState(*s.d.State), append([]runtime.Installed(nil), s.d.Runtimes...), append([]project.Project(nil), s.d.Projects...)
}

// ---- Reconcile ------------------------------------------------------------

// Reconcile leva o supervisor ao estado desejado:
//  1. desired() → specs/sites/pools/warnings
//  2. web.Render → etc/<web>.next → web.Validate(.next). Falhou? remove .next,
//     NÃO toca no supervisor, mantém etc/<web> anterior e retorna erro.
//  3. promove: WriteFiles(etc/<web>) (changed = algum arquivo mudou), apaga .next
//  4. php.ini por major em etc/php/<major>/php.ini (iniChanged por major)
//  5. diff de specs: remove os que sumiram, adiciona novos, substitui os que
//     mudaram; Restart do web se changed; Restart de workers cujo php.ini mudou
//  6. state.PortAlloc = alloc.Snapshot(); Save
//  7. hosts: se o conjunto de domínios difere do bloco atual, grava tmp em
//     var/run e chama o helper elevado (UAC negado → warning, não erro)
//  8. Emit("stack:warnings")
func (s *Stack) Reconcile(ctx context.Context) ([]Warning, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reconcileLocked(ctx)
}

func (s *Stack) reconcileLocked(ctx context.Context) ([]Warning, error) {
	st, rts, projs := s.snapshot()
	var warnings []Warning

	web := s.d.Web[st.WebServer]
	if web == nil {
		warnings = append(warnings, Warning{
			Code:    "web-missing",
			Message: fmt.Sprintf("web server %q não está instalado em bin/; nenhum site será servido", st.WebServer),
		})
	}

	tlsFn, tlsWarns := s.tlsIssuer()
	warnings = append(warnings, tlsWarns...)

	// 1. desejado
	out, err := desired(desiredInput{
		State: st, Runtimes: rts, Projects: projs, Alloc: s.d.Alloc, Web: web, TLS: tlsFn,
		EtcDir: paths.Etc(), VarDir: paths.Var(), LogDir: paths.Log(),
	})
	if err != nil {
		return s.finish(warnings), err
	}
	warnings = append(warnings, out.Warnings...)

	// 2–3. web server: render → .next → validate → promover
	webChanged := false
	if web != nil {
		webChanged, err = s.renderWeb(web, out, st)
		if err != nil {
			return s.finish(warnings), err
		}
		warnings = append(warnings, s.portConflicts(st, "web:"+string(web.Name()))...)
	}

	// 4. php.ini por major
	iniChanged := map[string]bool{}
	for _, pool := range out.Pools {
		inst, _ := runtime.PHPByMajor(runtime.ByKind(rts, runtime.PHP), pool.Version)
		changed, err := s.renderPHPIni(inst, pool.Version, out.Extensions[pool.Version])
		if err != nil {
			return s.finish(warnings), err
		}
		iniChanged[pool.Version] = changed
	}

	// 5. diff de specs
	if err := s.applySpecs(out.Specs, webChanged, iniChanged); err != nil {
		return s.finish(warnings), err
	}

	// 6. persistir portas
	if err := s.UpdateState(func(st *state.State) { st.PortAlloc = s.d.Alloc.Snapshot() }); err != nil {
		return s.finish(warnings), err
	}

	// 7. hosts
	hostWarns, err := s.syncHosts(out.Sites)
	warnings = append(warnings, hostWarns...)
	if err != nil {
		return s.finish(warnings), err
	}

	// 8. publicar
	return s.finish(warnings), nil
}

func (s *Stack) finish(w []Warning) []Warning {
	if w == nil {
		w = []Warning{}
	}
	s.stateMu.Lock()
	s.warnings = w
	s.stateMu.Unlock()
	s.d.Emit("stack:warnings", w)
	return w
}

// tlsIssuer devolve a função de emissão de certificados ou nil (+ warning) quando
// o mkcert não existe ou a CA não pôde ser instalada. A instalação da CA exige
// UAC uma única vez (helper mkcert-install); negação vira warning e sites ficam
// só em HTTP nesta sessão.
func (s *Stack) tlsIssuer() (func([]string) (string, string, error), []Warning) {
	mk := s.d.Mkcert
	if mk.Exe == "" {
		return nil, []Warning{{Code: "tls-unavailable", Message: "mkcert não encontrado em bin/mkcert/mkcert.exe; sites só em HTTP"}}
	}
	if !s.caReady {
		ok, err := mk.CAInstalled()
		if err != nil {
			return nil, []Warning{{Code: "tls-unavailable", Message: fmt.Sprintf("verificar CA do mkcert: %v", err)}}
		}
		if !ok {
			helper, herr := elevate.HelperPath()
			if herr != nil {
				return nil, []Warning{{Code: "tls-unavailable", Message: fmt.Sprintf("helper elevado indisponível: %v", herr)}}
			}
			err := elevate.RunElevated(helper, []string{"mkcert-install", "--exe", mk.Exe})
			switch {
			case errors.Is(err, elevate.ErrElevationDenied):
				return nil, []Warning{{Code: "elevation-denied", Message: "instalação da CA local (mkcert -install) cancelada; sites só em HTTP"}}
			case err != nil:
				return nil, []Warning{{Code: "tls-unavailable", Message: fmt.Sprintf("mkcert -install falhou: %v", err)}}
			}
		}
		s.caReady = true
	}
	return mk.IssueCert, nil
}

// renderWeb renderiza em etc/<name>.next, valida lá e só então grava em
// etc/<name>. Isso exige que Render() seja relocável (C6 — sem etcDir embutido).
// Retorna changed=true se algum arquivo do destino mudou.
func (s *Stack) renderWeb(web webserver.WebServer, out desiredOutput, st state.State) (bool, error) {
	name := string(web.Name())
	ports := webserver.Ports{HTTP: st.HTTPPort, HTTPS: st.HTTPSPort}
	files, err := web.Render(out.Sites, out.Pools, ports, filepath.ToSlash(paths.Log()))
	if err != nil {
		return false, fmt.Errorf("stack: renderizar %s: %w", name, err)
	}

	dest := filepath.Join(paths.Etc(), name)
	next := dest + ".next"
	if err := os.RemoveAll(next); err != nil {
		return false, fmt.Errorf("stack: limpar %s: %w", next, err)
	}
	if _, err := render.WriteFiles(next, files); err != nil {
		return false, fmt.Errorf("stack: gravar %s: %w", next, err)
	}
	ensureWebDirs(next)
	if err := web.Validate(next); err != nil {
		_ = os.RemoveAll(next)
		return false, fmt.Errorf("stack: config do %s inválida — a anterior foi mantida: %w", name, err)
	}

	changed, err := render.WriteFiles(dest, files)
	if err != nil {
		return false, fmt.Errorf("stack: promover %s: %w", dest, err)
	}
	ensureWebDirs(dest)
	_ = os.RemoveAll(next)
	return changed, nil
}

// ensureWebDirs cria os subdiretórios que o nginx exige dentro de -p (logs/ e
// temp/); inofensivo para o Apache. WriteFiles não apaga subdirs que não
// conhece, então eles sobrevivem a rerenders.
func ensureWebDirs(etcDir string) {
	_ = os.MkdirAll(filepath.Join(etcDir, "logs"), 0o755)
	_ = os.MkdirAll(filepath.Join(etcDir, "temp"), 0o755)
}

// renderPHPIni grava etc/php/<major>/php.ini se o conteúdo mudou.
func (s *Stack) renderPHPIni(inst runtime.Installed, major string, ext []string) (bool, error) {
	dir := filepath.Join(paths.Etc(), "php", major)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("stack: criar %s: %w", dir, err)
	}
	tmpDir := filepath.Join(paths.Var(), "tmp", "php")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return false, fmt.Errorf("stack: criar %s: %w", tmpDir, err)
	}
	content := render.RenderPHPIni(inst, ext, filepath.ToSlash(tmpDir), filepath.ToSlash(paths.Log()))
	changed, err := render.WriteFiles(dir, map[string][]byte{"php.ini": content})
	if err != nil {
		return false, fmt.Errorf("stack: gravar php.ini %s: %w", major, err)
	}
	return changed, nil
}

// portConflicts avisa se as portas do web server estão ocupadas por outro
// processo (ex.: httpd.exe do Laragon). Se o web:* já é nosso e está rodando,
// a porta ocupada é a nossa — sem aviso.
func (s *Stack) portConflicts(st state.State, webID string) []Warning {
	if status, ok := s.d.Sup.Status(webID); ok && isRunning(status.State) {
		return nil
	}
	var warns []Warning
	for _, port := range []int{st.HTTPPort, st.HTTPSPort} {
		if netcfg.IsFree(port) {
			continue
		}
		msg := fmt.Sprintf("porta %d ocupada", port)
		if pid, exe, err := netcfg.WhoHolds(port); err == nil {
			msg = fmt.Sprintf("porta %d ocupada por %s (PID %d)", port, filepath.Base(exe), pid)
		}
		warns = append(warns, Warning{Code: "port-conflict", Message: msg})
	}
	return warns
}

// applySpecs faz o diff entre applied e want. Um spec "mudou" quando Exe, Args,
// Env, Dir ou Port diferem (Probe é função; não comparável). Specs novos sobem
// imediatamente se StartAll já rodou.
func (s *Stack) applySpecs(want []supervisor.Spec, webChanged bool, iniChanged map[string]bool) error {
	wantByID := make(map[string]supervisor.Spec, len(want))
	for _, sp := range want {
		wantByID[sp.ID] = sp
	}

	// removidos
	for id := range s.applied {
		if _, keep := wantByID[id]; keep {
			continue
		}
		if err := s.d.Sup.Remove(id); err != nil {
			return fmt.Errorf("stack: remover %s: %w", id, err)
		}
		delete(s.applied, id)
		s.d.Logger.Info("stack: spec removido", "id", id)
	}

	// novos e alterados — em groupOrder para o start acontecer na ordem certa
	for _, id := range orderedIDs(want) {
		sp := wantByID[id]
		old, exists := s.applied[id]
		switch {
		case !exists:
			if err := s.d.Sup.Add(sp); err != nil {
				return fmt.Errorf("stack: adicionar %s: %w", id, err)
			}
			s.applied[id] = sp
			if s.started {
				if err := s.d.Sup.Start(id); err != nil {
					s.d.Logger.Warn("stack: start após add", "id", id, "err", err)
				}
			}
		case specChanged(old, sp):
			wasRunning := s.isRunning(id)
			if err := s.d.Sup.Remove(id); err != nil {
				return fmt.Errorf("stack: substituir %s: %w", id, err)
			}
			if err := s.d.Sup.Add(sp); err != nil {
				return fmt.Errorf("stack: readicionar %s: %w", id, err)
			}
			s.applied[id] = sp
			if wasRunning {
				if err := s.d.Sup.Start(id); err != nil {
					s.d.Logger.Warn("stack: start após substituir", "id", id, "err", err)
				}
			}
		default:
			// igual — talvez precise de restart por mudança de config
			restart := false
			if sp.Group == "web" && webChanged {
				restart = true
			}
			if sp.Group == "php" {
				parts := strings.Split(sp.ID, ":") // php:<major>:<i>
				if len(parts) == 3 && iniChanged[parts[1]] {
					restart = true
				}
			}
			if restart && s.isRunning(id) {
				if err := s.d.Sup.Restart(id); err != nil {
					s.d.Logger.Warn("stack: restart por config", "id", id, "err", err)
				}
			}
		}
	}
	return nil
}

func specChanged(a, b supervisor.Spec) bool {
	return a.Exe != b.Exe || a.Dir != b.Dir || a.Port != b.Port ||
		!reflect.DeepEqual(a.Args, b.Args) || !reflect.DeepEqual(a.Env, b.Env)
}

func isRunning(st supervisor.State) bool {
	return st == supervisor.Starting || st == supervisor.Ready || st == supervisor.Degraded
}

func (s *Stack) isRunning(id string) bool {
	st, ok := s.d.Sup.Status(id)
	return ok && isRunning(st.State)
}

// orderedIDs devolve os IDs em groupOrder e, dentro do grupo, por ID.
func orderedIDs(specs []supervisor.Spec) []string {
	rank := map[string]int{}
	for i, g := range groupOrder {
		rank[g] = i
	}
	sorted := append([]supervisor.Spec(nil), specs...)
	sort.Slice(sorted, func(i, j int) bool {
		ri, rj := rank[sorted[i].Group], rank[sorted[j].Group]
		if ri != rj {
			return ri < rj
		}
		return sorted[i].ID < sorted[j].ID
	})
	ids := make([]string, len(sorted))
	for i, sp := range sorted {
		ids[i] = sp.ID
	}
	return ids
}

// syncHosts compara o conjunto de domínios dos sites (sem "*." — hosts não
// suporta wildcard) com o bloco atual e, se diferir, pede ao helper elevado
// para gravar. Uma UAC por Reconcile no máximo; nenhuma se nada mudou.
func (s *Stack) syncHosts(sites []webserver.Site) ([]Warning, error) {
	want := make([]string, 0, len(sites))
	for _, site := range sites {
		want = append(want, site.Domain)
	}
	sort.Strings(want)

	current, err := os.ReadFile(netcfg.HostsPath)
	if err != nil {
		return nil, fmt.Errorf("stack: ler hosts: %w", err)
	}
	have := netcfg.ParseHostsBlock(string(current))
	sort.Strings(have)
	if reflect.DeepEqual(have, want) {
		return nil, nil
	}

	rendered := netcfg.RenderHostsBlock(string(current), want)
	if bytes.Equal([]byte(rendered), current) {
		return nil, nil
	}
	if err := os.MkdirAll(paths.Run(), 0o755); err != nil {
		return nil, fmt.Errorf("stack: criar %s: %w", paths.Run(), err)
	}
	tmp := filepath.Join(paths.Run(), "hosts.next")
	if err := os.WriteFile(tmp, []byte(rendered), 0o644); err != nil {
		return nil, fmt.Errorf("stack: gravar %s: %w", tmp, err)
	}
	helper, herr := elevate.HelperPath()
	if herr != nil {
		return nil, fmt.Errorf("stack: helper elevado indisponível: %w", herr)
	}
	err = elevate.RunElevated(helper, []string{"hosts-write", "--from", tmp})
	switch {
	case errors.Is(err, elevate.ErrElevationDenied):
		return []Warning{{
			Code:    "elevation-denied",
			Message: fmt.Sprintf("hosts não atualizado (UAC cancelado); domínios pendentes: %s", strings.Join(want, ", ")),
		}}, nil
	case err != nil:
		return nil, fmt.Errorf("stack: helper hosts-write: %w", err)
	}
	s.d.Logger.Info("stack: hosts atualizado", "domains", want)
	return nil, nil
}

// ---- start/stop -----------------------------------------------------------

// StartAll sobe tudo em groupOrder (php → web → db → mail → proc). Erros de um
// spec não impedem os demais (spec §13); o primeiro erro é retornado ao final.
func (s *Stack) StartAll(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var first error
	for _, id := range orderedIDs(s.appliedList()) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.isRunning(id) {
			continue
		}
		if err := s.d.Sup.Start(id); err != nil && first == nil {
			first = fmt.Errorf("stack: iniciar %s: %w", id, err)
		}
	}
	s.started = true
	return first
}

// StopAll para tudo na ordem inversa (proc → mail → db → web → php).
func (s *Stack) StopAll(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := orderedIDs(s.appliedList())
	var first error
	for i := len(ids) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !s.isRunning(ids[i]) {
			continue
		}
		if err := s.d.Sup.Stop(ids[i]); err != nil && first == nil {
			first = fmt.Errorf("stack: parar %s: %w", ids[i], err)
		}
	}
	s.started = false
	return first
}

func (s *Stack) appliedList() []supervisor.Spec {
	out := make([]supervisor.Spec, 0, len(s.applied))
	for _, sp := range s.applied {
		out = append(out, sp)
	}
	return out
}

// waitReady faz polling de Status até Ready, Failed/Stopped ou timeout.
// Start é assíncrono no supervisor (retorna em Starting).
func (s *Stack) waitReady(ctx context.Context, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		st, ok := s.d.Sup.Status(id)
		if !ok {
			return fmt.Errorf("stack: %s desapareceu do supervisor", id)
		}
		switch st.State {
		case supervisor.Ready:
			return nil
		case supervisor.Failed, supervisor.Stopped:
			return fmt.Errorf("stack: %s em estado %s: %s", id, st.State, st.LastError)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("stack: %s não ficou pronto em %s (estado %s)", id, timeout, st.State)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// ---- troca de web server --------------------------------------------------

// SwitchWebServer troca Apache↔nginx a quente:
//  1. renderiza o novo em etc/<novo>.next e valida — falhou? erro, nada mudou
//  2. para e remove web:<atual> (ambos querem 80/443)
//  3. promove .next → etc/<novo>; adiciona e inicia web:<novo>; espera Ready 20s
//  4. Ready → state.WebServer = novo, Save, Emit settings:changed, Reconcile
//     (regenera warnings como htaccess-under-nginx)
//  5. não ficou Ready → remove o novo, readiciona e inicia o anterior, retorna erro
func (s *Stack) SwitchWebServer(ctx context.Context, name state.WebServerName) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, rts, projs := s.snapshot()
	if name == st.WebServer {
		return nil
	}
	newWeb := s.d.Web[name]
	if newWeb == nil {
		return fmt.Errorf("stack: web server %q não está instalado", name)
	}

	// 1. desejado + render/validate do novo (sem mexer no alocador além do que já está)
	tlsFn, _ := s.tlsIssuer()
	stNew := st
	stNew.WebServer = name
	out, err := desired(desiredInput{
		State: stNew, Runtimes: rts, Projects: projs, Alloc: s.d.Alloc, Web: newWeb, TLS: tlsFn,
		EtcDir: paths.Etc(), VarDir: paths.Var(), LogDir: paths.Log(),
	})
	if err != nil {
		return err
	}
	if _, err := s.renderWeb(newWeb, out, stNew); err != nil {
		return err
	}
	var newSpec supervisor.Spec
	for _, sp := range out.Specs {
		if sp.ID == "web:"+string(name) {
			newSpec = sp
		}
	}
	if newSpec.ID == "" {
		return fmt.Errorf("stack: desired não produziu spec web:%s", name)
	}

	// 2. parar o atual
	oldID := "web:" + string(st.WebServer)
	oldSpec, hadOld := s.applied[oldID]
	oldRunning := hadOld && s.isRunning(oldID)
	if hadOld {
		if err := s.d.Sup.Remove(oldID); err != nil {
			return fmt.Errorf("stack: parar %s: %w", oldID, err)
		}
		delete(s.applied, oldID)
	}

	// 3. subir o novo
	if err := s.d.Sup.Add(newSpec); err != nil {
		return s.rollbackWeb(oldSpec, hadOld, oldRunning, fmt.Errorf("stack: adicionar %s: %w", newSpec.ID, err))
	}
	s.applied[newSpec.ID] = newSpec
	if err := s.d.Sup.Start(newSpec.ID); err != nil {
		return s.rollbackWeb(oldSpec, hadOld, oldRunning, fmt.Errorf("stack: iniciar %s: %w", newSpec.ID, err))
	}
	if err := s.waitReady(ctx, newSpec.ID, 20*time.Second); err != nil {
		return s.rollbackWeb(oldSpec, hadOld, oldRunning, err)
	}

	// 4. persistir e reconciliar (warnings, hosts inalterado)
	if err := s.UpdateState(func(st *state.State) { st.WebServer = name }); err != nil {
		return err
	}
	s.d.Emit("settings:changed", s.State())
	s.d.Logger.Info("stack: web server trocado", "to", name)
	_, err = s.reconcileLocked(ctx)
	return err
}

func (s *Stack) rollbackWeb(oldSpec supervisor.Spec, hadOld, oldRunning bool, cause error) error {
	for id := range s.applied {
		if strings.HasPrefix(id, "web:") {
			_ = s.d.Sup.Remove(id)
			delete(s.applied, id)
		}
	}
	if hadOld {
		if err := s.d.Sup.Add(oldSpec); err != nil {
			return fmt.Errorf("%w; rollback falhou ao readicionar %s: %v", cause, oldSpec.ID, err)
		}
		s.applied[oldSpec.ID] = oldSpec
		if oldRunning {
			if err := s.d.Sup.Start(oldSpec.ID); err != nil {
				return fmt.Errorf("%w; rollback falhou ao iniciar %s: %v", cause, oldSpec.ID, err)
			}
		}
	}
	return fmt.Errorf("troca de web server desfeita: %w", cause)
}
