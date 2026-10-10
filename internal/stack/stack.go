package stack

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hyphp/internal/elevate"
	"hyphp/internal/i18n"
	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
	"hyphp/internal/pkgmgr"
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
	// Catalog devolve o catálogo de pacotes em uso, e Fetch baixa um pacote
	// conferido pelo sha256, sem extrair. O stack os usa para o pacote de CAs
	// do PHP no Windows (cacert_windows.go).
	Catalog func() pkgmgr.Catalog
	Fetch   func(ctx context.Context, pkg pkgmgr.Package, dest string) error
	Logger  *slog.Logger
	Emit    func(name string, data any)
}

// Stack aplica o estado desejado ao supervisor.
//
// Locks: op serializa Reconcile/StartAll/StopAll/SwitchWebServer (operações
// longas, com I/O e UAC); stateMu protege *d.State para leituras rápidas dos
// services enquanto op está preso. Nunca segurar stateMu ao chamar o supervisor.
type Stack struct {
	// op é um semáforo de uma vaga, e não sync.Mutex, para StartAll, StopAll
	// e Close poderem desistir quando o ctx vence: no encerramento o StopAll
	// ficava preso atrás de um Reconcile longo (InitDBData chega a 180 s) e o
	// app não fechava. As demais operações esperam como antes.
	op      chan struct{}
	stateMu sync.RWMutex
	d       Deps

	applied  map[string]supervisor.Spec // specs hoje registrados no supervisor
	started  bool                       // StartAll já rodou → specs novos sobem no Reconcile
	warnings []Warning
	caReady  bool // mkcert -install já confirmado nesta sessão
	dbSync   bool // plano 07: sincronização de databases em voo
	// lastSites guarda os sites do último Reconcile para o ApplyHosts saber
	// quais domínios gravar sem recalcular o desired().
	lastSites []webserver.Site
	// resolver é o servidor DNS local. No Windows só sobe quando algum
	// projeto usa wildcard; no Mac fica sempre de pé, porque a regra
	// /etc/resolver/test manda todo o .test para ele.
	resolver *netcfg.Resolver
	// nrptDone marca que a regra NRPT desta sessão já foi registrada, para o
	// Reconcile parar de avisar. Só o Windows tem NRPT; no Mac a regra é
	// sempre lida do /etc/resolver/test.
	//lint:ignore U1000 usado só em wildcard_windows.go
	nrptDone bool
	// cacertFetching marca o download do pacote de CAs em voo: o Reconcile
	// roda várias vezes seguidas e não pode abrir um download a cada vez.
	//lint:ignore U1000 usado só em cacert_windows.go
	cacertFetching atomic.Bool
	// stoppedByDir são os serviços que StopUsingDir parou e que estavam no ar.
	// Quando a revarredura troca o spec para outra versão, applySpecs os
	// religa: o Stopped veio da remoção do runtime, não de um pedido do usuário.
	stoppedByDir map[string]bool
}

func New(d Deps) *Stack {
	return &Stack{d: d, op: make(chan struct{}, 1), applied: map[string]supervisor.Spec{}}
}

// lock toma a vaga de op sem prazo.
func (s *Stack) lock() { s.op <- struct{}{} }

func (s *Stack) unlock() { <-s.op }

// lockCtx toma a vaga de op ou desiste com ctx.Err() se o ctx vencer antes.
// O ctx é conferido primeiro: com ele já vencido e op livre, o select
// escolheria ao acaso entre os dois casos.
func (s *Stack) lockCtx(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case s.op <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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

// Projects devolve os projetos com PHPEffective já resolvido. A resolução é
// feita aqui, e não na varredura, porque depende do estado (DefaultPHP) e dos
// runtimes instalados — que mudam sem o projeto mudar.
func (s *Stack) Projects() []project.Project {
	s.stateMu.RLock()
	out := append([]project.Project(nil), s.d.Projects...)
	padrao := s.d.State.DefaultPHP
	phps := runtime.ByKind(s.d.Runtimes, runtime.PHP)
	s.stateMu.RUnlock()

	for i := range out {
		out[i].PHPEffective = ResolvePHPMajor(out[i].PHP, padrao, phps)
	}
	return out
}

func (s *Stack) SetProjects(p []project.Project) {
	s.stateMu.Lock()
	s.d.Projects = append([]project.Project(nil), p...)
	s.stateMu.Unlock()
}

// SetRuntimes troca a lista de runtimes conhecidos.
func (s *Stack) SetRuntimes(r []runtime.Installed) {
	s.stateMu.Lock()
	s.d.Runtimes = append([]runtime.Installed(nil), r...)
	s.stateMu.Unlock()
}

// SetWebServers troca os web servers disponíveis. Necessário porque eles são
// construídos a partir do que existe em bin/, e instalar o Apache com o app
// aberto deixava o mapa vazio até o próximo início — o Reconcile seguia
// avisando "web server não encontrado" com o Apache já instalado.
func (s *Stack) SetWebServers(w map[state.WebServerName]webserver.WebServer) {
	s.stateMu.Lock()
	s.d.Web = w
	s.stateMu.Unlock()
}

// SetMkcert troca o mkcert conhecido, pelo mesmo motivo de SetWebServers.
func (s *Stack) SetMkcert(mk netcfg.Mkcert) {
	s.stateMu.Lock()
	s.d.Mkcert = mk
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
	// PHPIni é mapa de mapas: copiar só o externo deixava o Reconcile
	// iterando o mapa interno (renderPHPIni) enquanto UpdateState o escrevia,
	// e escrita concorrente em mapa derruba o processo inteiro.
	if st.PHPIni != nil {
		pi := make(map[string]map[string]string, len(st.PHPIni))
		for k, v := range st.PHPIni {
			pi[k] = maps.Clone(v)
		}
		st.PHPIni = pi
	}
	return st
}

// snapshot copia o que desired precisa, sem segurar stateMu durante I/O.
func (s *Stack) snapshot() (state.State, []runtime.Installed, []project.Project) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return cloneState(*s.d.State), append([]runtime.Installed(nil), s.d.Runtimes...), append([]project.Project(nil), s.d.Projects...)
}

// webServer e mkcert leem sob stateMu porque SetWebServers/SetMkcert são
// chamados fora de op, com um Reconcile possivelmente em andamento; o
// Mkcert, struct de vários campos, podia sair rasgado (Exe de uma instância,
// CARoot de outra).
func (s *Stack) webServer(name state.WebServerName) webserver.WebServer {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.d.Web[name]
}

func (s *Stack) mkcert() netcfg.Mkcert {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.d.Mkcert
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
	s.lock()
	defer s.unlock()
	return s.reconcileLocked(ctx)
}

func (s *Stack) reconcileLocked(ctx context.Context) ([]Warning, error) {
	st, rts, projs := s.snapshot()
	var warnings []Warning

	web := s.webServer(st.WebServer)
	if web == nil {
		warnings = append(warnings, Warning{
			Code: "web-missing",
			// O caminho completo é obrigatório na mensagem: "não está instalado
			// em bin/" leva o usuário a olhar o bin/ errado quando existe outra
			// ferramenta na máquina com um Apache próprio.
			Message: i18n.T("warn.webMissing", st.WebServer, filepath.Join(paths.Bin(), string(st.WebServer))),
		})
	}

	tlsFn, tlsWarns := s.tlsIssuer()
	warnings = append(warnings, tlsWarns...)

	// 0. (plano 07) my.ini + datadir do MySQL antes de desired(): sem datadir
	// inicializado o runtime sai da lista e nenhum spec mysql é emitido.
	rts, myIniChanged, dbWarns := s.ensureMySQL(ctx, rts, st)
	warnings = append(warnings, dbWarns...)

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

	// 4. php.ini (e, no Mac, php-fpm.conf) por major. O bundle de CAs vem
	// antes: quando ele aparece, o php.ini muda e os pools reiniciam neste
	// mesmo Reconcile.
	caFile, caWarns := s.syncCACert(rts)
	warnings = append(warnings, caWarns...)
	iniChanged := map[string]bool{}
	sendmail := phpSendmail(rts, st.MailpitSMTPPort)
	for _, pool := range out.Pools {
		inst, _ := runtime.PHPByMajor(runtime.ByKind(rts, runtime.PHP), pool.Version)
		changed, err := s.renderPHPIni(inst, pool, st.PoolSize, out.Extensions[pool.Version], st.MailpitSMTPPort, sendmail, caFile, st.PHPIni[pool.Version])
		if err != nil {
			return s.finish(warnings), err
		}
		iniChanged[pool.Version] = changed
	}

	// 4b. config.inc.php da ferramenta, dentro da própria instalação
	if out.Tool != nil {
		if err := s.renderPhpMyAdmin(out.Tool, st); err != nil {
			warnings = append(warnings, Warning{Code: "pma-config-failed", Message: err.Error()})
		}
	}

	// 5. diff de specs
	if err := s.applySpecs(out.Specs, webChanged, iniChanged, myIniChanged); err != nil {
		return s.finish(warnings), err
	}

	// 6. persistir portas
	if err := s.UpdateState(func(st *state.State) { st.PortAlloc = s.d.Alloc.Snapshot() }); err != nil {
		return s.finish(warnings), err
	}

	// 7. hosts — só detecta a pendência; a escrita (e a UAC) é sob demanda,
	// via ApplyHosts, disparada pela UI.
	// Docroot sem índice é a causa silenciosa mais comum de "o domínio abre um
	// 404": o vhost casa, o servidor entra no diretório e não acha o que
	// servir. Sem este aviso resta adivinhar entre DNS, vhost, PHP e caminho.
	for _, p := range projs {
		if !p.HasIndex {
			warnings = append(warnings, Warning{
				Code:      "docroot-sem-indice",
				ProjectID: p.ID,
				Message:   i18n.T("warn.docrootSemIndice", p.Domain, p.DocrootAbs),
			})
		}
	}

	s.lastSites = append(s.lastSites[:0], out.Sites...)
	if manageHosts() {
		hostWarns, err := s.syncHosts(out.Sites)
		warnings = append(warnings, hostWarns...)
		if err != nil {
			return s.finish(warnings), err
		}
	}

	// 7b. DNS wildcard: o resolvedor sobe sem privilégio; a regra do sistema,
	// que exige senha, fica para ApplyWildcardDNS: no Windows é a NRPT (UAC),
	// no Mac é o /etc/resolver/test, gravado pelo helper com a senha.
	warnings = append(warnings, s.syncWildcard(projs)...)
	// 7c. lista da página de host sem projeto, depois do hosts e do DNS: ela
	// diz se cada domínio abre nesta máquina.
	if web != nil {
		s.writePageData(web, out.Sites, st)
	}
	// 8. (plano 07) databases dos manifestos, em background: espera o mysql
	// ficar ready sem segurar o Reconcile.
	s.syncDatabases(rts, projs)

	// 9. publicar
	return s.finish(warnings), nil
}

func (s *Stack) finish(w []Warning) []Warning {
	if w == nil {
		w = []Warning{}
	}
	s.stateMu.Lock()
	s.warnings = w
	s.stateMu.Unlock()
	// Os warnings também vão para o log. Sem isto eles existem só na tela, e
	// um diagnóstico como "porta 80 ocupada por outro processo" fica invisível
	// para quem está lendo o log — que é exatamente onde se procura quando o
	// web server entra em ciclo de reinício e a única linha visível é
	// "processo encerrou com código 1".
	for _, warn := range w {
		s.d.Logger.Warn("stack: aviso", "code", warn.Code, "msg", warn.Message, "project", warn.ProjectID)
	}
	s.d.Emit("stack:warnings", w)
	return w
}

// tlsIssuer devolve a função de emissão de certificados ou nil (+ warning)
// quando o mkcert não existe ou a CA local ainda não foi instalada.
//
// Instalar a CA exige UAC, e pela mesma razão do hosts (C18.42) isso NÃO pode
// acontecer aqui: tlsIssuer roda dentro do Reconcile, que roda no bootstrap.
// Um usuário novo — que é justamente quem ainda não tem a CA — abriria o app
// e receberia um diálogo de UAC antes da primeira tela. Sem CA os sites saem
// só em HTTP e o warning "ca-pending" leva ao botão que instala.
func (s *Stack) tlsIssuer() (func([]string) (string, string, error), []Warning) {
	mk := s.mkcert()
	if mk.Exe == "" {
		return nil, []Warning{{Code: "tls-unavailable", Message: mkcertMissingMessage()}}
	}
	if !s.caReady {
		ok, err := mk.CAInstalled()
		if err != nil {
			return nil, []Warning{{Code: "tls-unavailable", Message: i18n.T("warn.caCheck", err)}}
		}
		if !ok {
			return nil, []Warning{{
				Code:    "ca-pending",
				Message: i18n.T("warn.caPending"),
			}}
		}
		s.caReady = true
	}
	return mk.IssueCert, nil
}

// InstallCA instala o certificado raiz local (mkcert -install) pelo helper
// elevado. Como ApplyHosts, é ação explícita do usuário — a única forma de
// habilitar HTTPS, e a única que pede UAC por causa de TLS.
func (s *Stack) InstallCA(ctx context.Context) error {
	mk := s.mkcert()
	if mk.Exe == "" {
		return mkcertMissingError()
	}
	switch ok, err := mk.CAInstalled(); {
	case err != nil:
		return i18n.Errorf("err.stack.caCheck", err)
	case ok:
		return nil
	}
	if err := s.trustCA(mk); err != nil {
		return err
	}
	s.d.Logger.Info("stack: CA local instalada")
	// Agora os vhosts podem nascer com TLS: o Reconcile re-emite tudo.
	_, err := s.Reconcile(ctx)
	return err
}

// renderWeb renderiza em etc/<name>.next, valida lá e só então grava em
// etc/<name>. Isso exige que Render() seja relocável (C6 — sem etcDir embutido).
// Retorna changed=true se algum arquivo do destino mudou.
//
// A cópia validada escuta em portas livres, e não nas reais: o nginx -t faz
// bind() de cada listen, e o Apache no Windows segura as portas com
// SO_EXCLUSIVEADDRUSE. Validar nas reais fazia a troca Apache → nginx falhar
// sempre, com erro 10013, antes de o Apache ser parado. As duas cópias só
// diferem nos números de porta; conflito real de porta aparece na subida, e
// a troca se desfaz.
func (s *Stack) renderWeb(web webserver.WebServer, out desiredOutput, st state.State) (bool, error) {
	name := string(web.Name())
	logDir := filepath.ToSlash(paths.Log())
	ports := webserver.Ports{HTTP: st.HTTPPort, HTTPS: st.HTTPSPort}
	files, err := web.Render(out.Sites, out.Pools, ports, logDir, out.Tool)
	if err != nil {
		return false, fmt.Errorf("stack: renderizar %s: %w", name, err)
	}
	free, err := freeLoopbackPorts(3)
	if err != nil {
		return false, fmt.Errorf("stack: portas para validar %s: %w", name, err)
	}
	checkTool := out.Tool
	if checkTool != nil {
		t := *checkTool
		t.Port = free[2]
		checkTool = &t
	}
	check, err := web.Render(out.Sites, out.Pools, webserver.Ports{HTTP: free[0], HTTPS: free[1]}, logDir, checkTool)
	if err != nil {
		return false, fmt.Errorf("stack: renderizar %s: %w", name, err)
	}

	dest := filepath.Join(paths.Etc(), name)
	next := dest + ".next"
	if err := os.RemoveAll(next); err != nil {
		return false, fmt.Errorf("stack: limpar %s: %w", next, err)
	}
	if _, err := render.WriteFiles(next, check); err != nil {
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

// freeLoopbackPorts devolve n portas distintas que o sistema deu como livres
// em 127.0.0.1. Os listeners ficam abertos até o último ser criado, para que
// o sistema não devolva a mesma porta duas vezes. Loopback, e não 0.0.0.0,
// para não disparar o aviso do Firewall do Windows no hyphp.exe.
func freeLoopbackPorts(n int) ([]int, error) {
	ports := make([]int, 0, n)
	for range n {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		defer ln.Close()
		ports = append(ports, ln.Addr().(*net.TCPAddr).Port)
	}
	return ports, nil
}

// ensureWebDirs cria os subdiretórios que o nginx exige dentro de -p (logs/ e
// temp/); inofensivo para o Apache. WriteFiles não apaga subdirs que não
// conhece, então eles sobrevivem a rerenders.
func ensureWebDirs(etcDir string) {
	_ = os.MkdirAll(filepath.Join(etcDir, "logs"), 0o755)
	_ = os.MkdirAll(filepath.Join(etcDir, "temp"), 0o755)
}

// renderPHPIni grava etc/php/<major>/ (php.ini e, no Mac, php-fpm.conf) se o
// conteúdo mudou. Os dois arquivos vão no mesmo WriteFiles: changed cobre
// php.ini, porta e PoolSize, e o iniChanged da série reinicia o php-fpm.
func (s *Stack) renderPHPIni(inst runtime.Installed, pool webserver.PHPPool, poolSize int, ext []string, smtpPort int, sendmail, caFile string, userIni map[string]string) (bool, error) {
	major := pool.Version
	dir := filepath.Join(paths.Etc(), "php", major)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("stack: criar %s: %w", dir, err)
	}
	tmpDir := filepath.Join(paths.Var(), "tmp", "php")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return false, fmt.Errorf("stack: criar %s: %w", tmpDir, err)
	}
	content := render.RenderPHPIni(inst, ext, filepath.ToSlash(tmpDir), filepath.ToSlash(paths.Log()), smtpPort, sendmail, mysqlSocket(paths.Var()), caFile, userIni)
	changed, err := render.WriteFiles(dir, phpConfFiles(content, pool, poolSize, paths.Log()))
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
		msg := i18n.T("warn.portBusy", port)
		if pid, exe, err := netcfg.WhoHolds(port); err == nil {
			msg = i18n.T("warn.portBusyBy", port, filepath.Base(exe), pid)
		}
		warns = append(warns, Warning{Code: "port-conflict", Message: msg})
	}
	return warns
}

// applySpecs faz o diff entre applied e want. Um spec "mudou" quando Exe, Args,
// Env, Dir ou Port diferem (Probe é função; não comparável). Specs novos sobem
// imediatamente se StartAll já rodou. dbChanged (plano 07) reinicia o mysql
// quando o my.ini muda — a linha de comando é a mesma, só o arquivo mudou.
func (s *Stack) applySpecs(want []supervisor.Spec, webChanged bool, iniChanged map[string]bool, dbChanged bool) error {
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
		delete(s.stoppedByDir, id)
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
			wasRunning := s.isRunning(id) || s.stoppedByDir[id]
			delete(s.stoppedByDir, id)
			// Replace, e não Remove+Add: o serviço trocado mantém a posição
			// no supervisor, cujo StopAll (usado pelo Close no encerramento)
			// segue essa ordem para parar dependentes antes das bases.
			if err := s.d.Sup.Replace(sp); err != nil {
				return fmt.Errorf("stack: substituir %s: %w", id, err)
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
			if sp.ID == MySQLSpecID && dbChanged {
				restart = true
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

// pendingHosts compara os domínios dos sites com o bloco atual do hosts e
// devolve o conteúdo a gravar. NÃO eleva e NÃO escreve: é chamada no Reconcile,
// que roda no boot e a cada mudança de projeto.
//
// Por que separada de ApplyHosts: escrever no hosts exige UAC, e pedir UAC no
// boot trava o app antes da primeira tela — quem só quer ver o estado dos
// serviços não deveria precisar de privilégio de administrador (spec §11).
// Pior: o Reconcile dispara várias vezes (watcher de projetos, de bin/), e cada
// chamada enfileirava um diálogo novo — chegaram a empilhar três. Agora o
// Reconcile só avisa, e a UI oferece a ação.
func (s *Stack) pendingHosts(sites []webserver.Site) (rendered string, have, want []string, pending bool, err error) {
	want = make([]string, 0, len(sites))
	for _, site := range sites {
		want = append(want, site.Domain)
	}
	sort.Strings(want)

	current, err := os.ReadFile(netcfg.HostsPath)
	if err != nil {
		return "", nil, nil, false, fmt.Errorf("stack: ler hosts: %w", err)
	}
	have = netcfg.ParseHostsBlock(string(current))
	sort.Strings(have)
	if reflect.DeepEqual(have, want) {
		return "", have, want, false, nil
	}
	rendered = netcfg.RenderHostsBlock(string(current), want)
	if bytes.Equal([]byte(rendered), current) {
		return "", have, want, false, nil
	}
	return rendered, have, want, true, nil
}

// hostsChange descreve a pendência nos dois sentidos. Só "o que falta" não
// basta: apagar o último projeto deixa want vazio e a mensagem saía
// "domínios ainda não estão no hosts: " seguida de nada — um pedido de UAC
// sem motivo visível.
func hostsChange(have, want []string) string {
	var add, rem []string
	for _, d := range want {
		if !slices.Contains(have, d) {
			add = append(add, d)
		}
	}
	for _, d := range have {
		if !slices.Contains(want, d) {
			rem = append(rem, d)
		}
	}
	var parts []string
	if len(add) > 0 {
		parts = append(parts, i18n.T("warn.hostsAdd", strings.Join(add, ", ")))
	}
	if len(rem) > 0 {
		parts = append(parts, i18n.T("warn.hostsRemove", strings.Join(rem, ", ")))
	}
	if len(parts) == 0 {
		return i18n.T("warn.hostsRewrite")
	}
	return i18n.T("warn.hostsUpdate", strings.Join(parts, "; "))
}

// syncHosts só REPORTA a pendência; a escrita acontece em ApplyHosts.
func (s *Stack) syncHosts(sites []webserver.Site) ([]Warning, error) {
	_, have, want, pending, err := s.pendingHosts(sites)
	if err != nil {
		return nil, err
	}
	if !pending {
		return nil, nil
	}
	return []Warning{{
		Code:    "hosts-pending",
		Message: hostsChange(have, want),
	}}, nil
}

// ApplyHosts grava o bloco do hosts pelo helper elevado. É a ÚNICA porta que
// dispara UAC por causa de domínios, e só é chamada por ação explícita do
// usuário (SettingsService.ApplyHosts → botão na UI).
func (s *Stack) ApplyHosts(ctx context.Context) error {
	if !manageHosts() {
		return errors.New(i18n.T("err.stack.hostsMac"))
	}
	s.lock()
	sites := append([]webserver.Site(nil), s.lastSites...)
	s.unlock()

	rendered, have, want, pending, err := s.pendingHosts(sites)
	if err != nil {
		return err
	}
	if !pending {
		return nil
	}
	if err := os.MkdirAll(paths.Run(), 0o755); err != nil {
		return fmt.Errorf("stack: criar %s: %w", paths.Run(), err)
	}
	tmp := filepath.Join(paths.Run(), "hosts.next")
	if err := os.WriteFile(tmp, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("stack: gravar %s: %w", tmp, err)
	}
	helper, herr := elevate.HelperPath()
	if herr != nil {
		return fmt.Errorf("stack: helper elevado indisponível: %w", herr)
	}
	switch err := elevate.RunElevated(helper, []string{"hosts-write", "--from", tmp}); {
	case errors.Is(err, elevate.ErrElevationDenied):
		return i18n.Errorf("err.stack.hostsCancelled", hostsChange(have, want))
	case err != nil:
		return fmt.Errorf("stack: helper hosts-write: %w", err)
	}
	s.d.Logger.Info("stack: hosts atualizado", "domains", want)
	// Sem reconciliar, Warnings() continua devolvendo o resultado do último
	// Reconcile — com o hosts-pending que acabou de deixar de existir — e o
	// card Permissões seguiria oferecendo uma ação já concluída.
	_, err = s.Reconcile(ctx)
	return err
}

// ---- start/stop -----------------------------------------------------------

// StartAll sobe tudo em groupOrder (php → web → db → mail → proc). Erros de um
// spec não impedem os demais (spec §13); o primeiro erro é retornado ao final.
// Desiste com ctx.Err() se o ctx vencer esperando outra operação terminar.
func (s *Stack) StartAll(ctx context.Context) error {
	if err := s.lockCtx(ctx); err != nil {
		return err
	}
	defer s.unlock()
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
	clear(s.stoppedByDir)
	return first
}

// StopAll para tudo na ordem inversa (proc → mail → db → web → php). Desiste
// com ctx.Err() se o ctx vencer esperando outra operação terminar: no
// encerramento quem garante a morte dos serviços é o Close do supervisor.
func (s *Stack) StopAll(ctx context.Context) error {
	if err := s.lockCtx(ctx); err != nil {
		return err
	}
	defer s.unlock()
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
	clear(s.stoppedByDir)
	return first
}

// StopUsingDir para os serviços cujo executável mora em dir. Apagar um runtime
// com o processo dele vivo falha no Windows ("Access is denied" na primeira DLL
// carregada), e o restart automático o traria de volta no meio da remoção. Os
// specs continuam registrados: a revarredura de bin/ depois da remoção dispara
// o Reconcile que troca de versão ou tira o serviço. Os que estavam no ar
// ficam em stoppedByDir para a troca de versão subi-los de novo.
func (s *Stack) StopUsingDir(dir string) error {
	s.lock()
	defer s.unlock()
	var first error
	for _, id := range specsUsingDir(s.appliedList(), dir) {
		if s.isRunning(id) {
			if s.stoppedByDir == nil {
				s.stoppedByDir = map[string]bool{}
			}
			s.stoppedByDir[id] = true
		}
		if err := s.d.Sup.Stop(id); err != nil && first == nil {
			first = fmt.Errorf("stack: parar %s: %w", id, err)
		}
	}
	return first
}

// specsUsingDir devolve, ordenados, os IDs dos specs cujo executável está
// dentro de dir. A comparação ignora maiúsculas, como o sistema de arquivos do
// Windows, e exige o separador depois de dir: "mysql-8.4.11-winx64-old" não
// está dentro de "mysql-8.4.11-winx64".
func specsUsingDir(specs []supervisor.Spec, dir string) []string {
	base := strings.ToLower(filepath.Clean(dir)) + string(filepath.Separator)
	var ids []string
	for _, sp := range specs {
		if strings.HasPrefix(strings.ToLower(filepath.Clean(sp.Exe)), base) {
			ids = append(ids, sp.ID)
		}
	}
	sort.Strings(ids)
	return ids
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
			return i18n.Errorf("err.stack.serviceGone", id)
		}
		// O nome ("MariaDB 11.4.13") diz mais do que o ID, que no banco é
		// "mysql" para os dois motores; a última falha do processo mostra
		// por que ele não ficou pronto sem precisar abrir o log.
		name := cmp.Or(st.Name, id)
		switch st.State {
		case supervisor.Ready:
			return nil
		case supervisor.Failed, supervisor.Stopped:
			return i18n.Errorf("err.stack.serviceState", name, st.State, st.LastError)
		}
		if time.Now().After(deadline) {
			if st.LastError != "" {
				return i18n.Errorf("err.stack.notReadyCause", name, timeout, st.State, st.LastError)
			}
			return i18n.Errorf("err.stack.notReady", name, timeout, st.State)
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
//  1. renderWeb valida uma cópia do novo em etc/<novo>.next e, só se passar,
//     grava etc/<novo> — falhou? erro, nada mudou
//  2. para e remove web:<atual> (ambos querem 80/443)
//  3. adiciona e inicia web:<novo>; espera Ready 20s
//  4. Ready → state.WebServer = novo, Save, Emit settings:changed, Reconcile
//     (regenera warnings como htaccess-under-nginx)
//  5. não ficou Ready → remove o novo, readiciona e inicia o anterior, retorna
//     erro. etc/<novo> fica gravado: o web server anterior não lê esse diretório.
func (s *Stack) SwitchWebServer(ctx context.Context, name state.WebServerName) error {
	s.lock()
	defer s.unlock()

	st, rts, projs := s.snapshot()
	if name == st.WebServer {
		return nil
	}
	newWeb := s.webServer(name)
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
	return i18n.Errorf("err.stack.webSwitchUndone", cause)
}

// ---- troca de banco -------------------------------------------------------

// dbReadyTimeout cobre a primeira subida de um motor: o Reconcile já
// inicializou o datadir, e falta o servidor abrir a porta.
const dbReadyTimeout = 60 * time.Second

// SwitchDatabase troca o motor de banco (state.DBMySQL ↔ state.DBMariaDB). O
// state passa a apontar para o novo motor e o Reconcile faz o resto: grava o
// my.ini dele, inicializa o datadir na primeira vez e troca o servidor do
// spec "mysql", que para um e sobe o outro na mesma porta. Cada motor fica
// com os próprios dados, sem cópia de um para o outro.
//
// Se o novo não ficar pronto, volta o state e reconcilia de novo: o banco
// que estava no ar volta, como na troca de web server.
func (s *Stack) SwitchDatabase(ctx context.Context, engine string) error {
	s.lock()
	defer s.unlock()
	if engine != state.DBMySQL && engine != state.DBMariaDB {
		return i18n.Errorf("err.settings.dbEngine", engine)
	}
	st, rts, _ := s.snapshot()
	target := runtime.Kind(engine)
	if _, ok := DBRuntime(rts, state.State{DBEngine: engine}); !ok {
		return i18n.Errorf("err.stack.dbEngineMissing", DBName(target))
	}
	cur, installed := DBRuntime(rts, st)
	if installed && cur.Kind == target {
		// Mesmo motor (inclusive o "vazio" que já resolvia para ele): só
		// grava a escolha, sem reiniciar nada.
		return s.UpdateState(func(st *state.State) { st.DBEngine = engine })
	}
	prev := st.DBEngine
	if err := s.UpdateState(func(st *state.State) { st.DBEngine = engine }); err != nil {
		return err
	}
	s.d.Emit("settings:changed", s.State())
	_, err := s.reconcileLocked(ctx)
	if err == nil && s.started {
		err = s.waitReady(ctx, MySQLSpecID, dbReadyTimeout)
	}
	if err == nil {
		s.d.Logger.Info("stack: banco trocado", "to", engine)
		return nil
	}
	if uerr := s.UpdateState(func(st *state.State) { st.DBEngine = prev }); uerr != nil {
		return fmt.Errorf("%w; e não deu para voltar o state: %v", err, uerr)
	}
	s.d.Emit("settings:changed", s.State())
	if _, rerr := s.reconcileLocked(ctx); rerr != nil {
		return fmt.Errorf("%w; e o Reconcile de volta falhou: %v", err, rerr)
	}
	back := "—"
	if installed {
		back = DBName(cur.Kind)
	}
	return i18n.Errorf("err.stack.dbSwitchUndone", back, err)
}

// renderPhpMyAdmin grava o config.inc.php dentro da instalação do phpMyAdmin.
//
// O arquivo mora junto do código, e não em etc/, porque é lá que o phpMyAdmin
// o procura — ele não aceita caminho externo sem variável de ambiente, que o
// FastCGI não carregaria. Some junto com a ferramenta quando ela é removida,
// o que também evita config órfã.
//
// O blowfish_secret nasce na primeira gravação e fica no state: gerar outro a
// cada Reconcile derrubaria a sessão aberta do usuário a cada mudança de
// projeto.
func (s *Stack) renderPhpMyAdmin(tool *webserver.Tool, st state.State) error {
	secret := st.PhpMyAdminSecret
	if len(secret) != 32 {
		novo, err := render.NewBlowfishSecret()
		if err != nil {
			return fmt.Errorf("stack: segredo do phpMyAdmin: %w", err)
		}
		secret = novo
		if err := s.UpdateState(func(cur *state.State) { cur.PhpMyAdminSecret = novo }); err != nil {
			return fmt.Errorf("stack: persistir segredo do phpMyAdmin: %w", err)
		}
	}

	tmp := filepath.Join(paths.Var(), "tmp", "phpmyadmin")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return fmt.Errorf("stack: criar %s: %w", tmp, err)
	}
	conf := render.PhpMyAdminConfig(st.MySQLPort, secret, filepath.ToSlash(tmp))
	// os.WriteFile direto, NUNCA render.WriteFiles: aquela função sincroniza o
	// diretório inteiro e apaga o que não está no mapa. Usada aqui, ela apagou
	// a instalação do phpMyAdmin e deixou só o config recém-escrito. Ela serve
	// a diretórios que o HyPHP possui por completo (etc/apache, etc/nginx);
	// este pertence ao pacote baixado.
	alvo := filepath.Join(filepath.FromSlash(tool.Docroot), "config.inc.php")
	if err := os.WriteFile(alvo, conf, 0o644); err != nil {
		return fmt.Errorf("stack: gravar %s: %w", alvo, err)
	}
	return nil
}
