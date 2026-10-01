package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// ErrClosed é devolvido por Start e Add depois de Close().
var ErrClosed = errors.New("supervisor fechado")

// ErrNotStopped: Start foi pedido para um serviço que não está parado. É
// sentinela porque StartAll precisa distinguir "já estava no ar" (que para uma
// operação de convergência é sucesso) de uma falha real ao subir.
var ErrNotStopped = errors.New("serviço não está parado")

// ErrRemoving: Start foi pedido para um serviço que Remove ou Replace está
// tirando do ar. Subir ali deixaria um processo vivo numa entry prestes a
// sair do mapa — fora de List(), com Stop respondendo "desconhecido" — até o
// app fechar. StartAll o trata como ErrNotStopped: o serviço está de saída.
var ErrRemoving = errors.New("serviço sendo removido")

// entry é o estado interno de um serviço. Tudo aqui é lido e escrito com
// s.mu preso, exceto spec e ring, que são imutáveis depois de Add.
type entry struct {
	spec     Spec
	status   Status
	ring     *LogRing
	cmd      *exec.Cmd
	job      windows.Handle
	cancel   context.CancelFunc
	restarts int
	logFile  *os.File
	done     chan struct{} // fechado quando o laço run() termina
	// stopping existe enquanto um Stop está em curso e é fechado quando ele
	// termina. Quem o cria é o único dono da parada: um segundo Stop só espera.
	stopping chan struct{}
	// removing existe enquanto um Remove ou Replace está em curso e é fechado
	// quando a entry sai do mapa (ou é trocada). Start recusa a entry nesse
	// meio-tempo; um segundo Remove/Replace espera e olha o mapa de novo.
	removing chan struct{}
}

// Supervisor mantém a máquina de estados de todos os serviços.
type Supervisor struct {
	mu        sync.Mutex
	entries   map[string]*entry
	order     []string
	subs      map[int]chan Event
	nextSub   int
	logger    *slog.Logger
	globalJob windows.Handle
	closed    bool
}

// New cria o supervisor e garante o Job Object global kill-on-close.
func New(logger *slog.Logger) (*Supervisor, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	job, err := AttachSelfToKillOnCloseJob()
	if err != nil {
		return nil, fmt.Errorf("job object global: %w", err)
	}
	return &Supervisor{
		entries:   make(map[string]*entry),
		subs:      make(map[int]chan Event),
		logger:    logger,
		globalJob: job,
	}, nil
}

// Add registra um serviço parado.
func (s *Supervisor) Add(spec Spec) error {
	if err := normalizeSpec(&spec); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if _, dup := s.entries[spec.ID]; dup {
		return fmt.Errorf("spec %s: ID duplicado", spec.ID)
	}
	s.entries[spec.ID] = newEntry(spec)
	s.order = append(s.order, spec.ID)
	return nil
}

// normalizeSpec valida e aplica os defaults de probe aqui para o laço run()
// não precisar decidir nada em tempo de execução — e porque
// time.NewTicker(0) entra em pânico.
func normalizeSpec(spec *Spec) error {
	if spec.ID == "" {
		return errors.New("spec sem ID")
	}
	if spec.Exe == "" {
		return fmt.Errorf("spec %s: Exe vazio", spec.ID)
	}
	if spec.Probe == nil {
		return fmt.Errorf("spec %s: Probe obrigatório", spec.ID)
	}
	if spec.ProbeInterval <= 0 {
		spec.ProbeInterval = defaultProbeInterval
	}
	if spec.ProbeTimeout <= 0 {
		spec.ProbeTimeout = defaultProbeTimeout
	}
	if spec.Name == "" {
		spec.Name = spec.ID
	}
	return nil
}

func newEntry(spec Spec) *entry {
	return &entry{
		spec: spec,
		ring: NewLogRing(ringCapacity),
		status: Status{
			ID:    spec.ID,
			Name:  spec.Name,
			Group: spec.Group,
			Port:  spec.Port,
			State: Stopped,
		},
	}
}

// Remove para o serviço (se estiver rodando), o esquece e publica um Event
// com Removed: quem acompanha a lista por eventos não tem outro jeito de saber
// que o "stopped" final foi de uma remoção.
func (s *Supervisor) Remove(id string) error {
	e, waited := s.claim(id)
	if e == nil {
		if waited { // outro Remove chegou primeiro e já publicou
			return nil
		}
		return fmt.Errorf("serviço desconhecido: %s", id)
	}
	stopErr := s.Stop(id)
	s.mu.Lock()
	last := e.status
	delete(s.entries, id)
	for i, cur := range s.order {
		if cur == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	close(e.removing)
	chans := s.subscribersLocked()
	s.mu.Unlock()
	s.broadcast(chans, Event{Status: last, Removed: true})
	return stopErr
}

// Replace para o serviço e troca o spec dele, que fica parado, na MESMA
// posição de s.order. Remove+Add o mandava para o fim, e StopAll (que o
// Close usa no encerramento) passava a parar o PHP antes do web server e os
// procs depois do banco. O ring de logs é mantido: quem acompanha os logs do
// serviço continua lendo o mesmo. Publica o retrato novo (nome e porta podem
// ter mudado) em vez de Removed, porque o serviço continua na lista.
func (s *Supervisor) Replace(spec Spec) error {
	if err := normalizeSpec(&spec); err != nil {
		return err
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return ErrClosed
	}
	e, _ := s.claim(spec.ID)
	if e == nil {
		return fmt.Errorf("serviço desconhecido: %s", spec.ID)
	}
	stopErr := s.Stop(spec.ID)
	s.mu.Lock()
	ne := newEntry(spec)
	ne.ring = e.ring
	s.entries[spec.ID] = ne
	close(e.removing)
	ev := Event{Status: ne.status}
	chans := s.subscribersLocked()
	s.mu.Unlock()
	s.broadcast(chans, ev)
	return stopErr
}

// claim reserva a entry para Remove/Replace marcando removing sob o lock,
// ANTES de parar: assim nenhum Start acha a entry Stopped entre a parada e a
// saída do mapa. Se outra remoção já está em curso, espera ela terminar e
// olha o mapa de novo; waited diz se houve essa espera.
func (s *Supervisor) claim(id string) (*entry, bool) {
	waited := false
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		e, ok := s.entries[id]
		if !ok {
			return nil, waited
		}
		if e.removing == nil {
			e.removing = make(chan struct{})
			return e, waited
		}
		ch := e.removing
		s.mu.Unlock()
		<-ch
		s.mu.Lock()
		waited = true
	}
}

// Start inicia o serviço e devolve assim que o processo nasce: o estado já é
// Starting; Ready ou Failed chegam depois por Subscribe(). Erro síncrono só
// para id desconhecido, serviço já em execução, supervisor fechado ou exe que
// não inicia (nesse caso o estado vai a Failed).
func (s *Supervisor) Start(id string) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	e, ok := s.entries[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("serviço desconhecido: %s", id)
	}
	if e.removing != nil {
		s.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrRemoving, id)
	}
	switch e.status.State {
	case Stopped, Failed:
	default:
		cur := e.status.State
		s.mu.Unlock()
		return fmt.Errorf("%w: serviço %s já está %s", ErrNotStopped, id, cur)
	}
	// A transição é reservada ainda sob o lock: um Start concorrente cai no
	// default acima em vez de criar um segundo processo.
	e.status.State = Starting
	e.status.LastError = ""
	e.status.PID = 0
	e.status.Restarts = 0
	e.status.StartedAt = time.Time{}
	e.restarts = 0
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.done = make(chan struct{})
	done := e.done
	ev := Event{Status: e.status}
	chans := s.subscribersLocked()
	s.mu.Unlock()
	s.broadcast(chans, ev)

	if err := s.launch(e); err != nil {
		cancel()
		close(done)
		s.setState(e, Failed, err)
		return err
	}
	go s.run(e, ctx)
	return nil
}

// Stop bloqueia até o serviço estar Stopped.
func (s *Supervisor) Stop(id string) error {
	s.mu.Lock()
	e, ok := s.entries[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("serviço desconhecido: %s", id)
	}
	if e.stopping != nil {
		// Outro Stop já está parando. Repetir o stopProcess usaria o job que o
		// dono fecha em releaseProcess, e no Windows o valor de um handle
		// fechado é reciclado: o TerminateJobObject podia matar outro serviço.
		stopping := e.stopping
		s.mu.Unlock()
		<-stopping
		return nil
	}
	switch e.status.State {
	case Stopped, Failed:
		s.mu.Unlock()
		return nil
	}
	stopping := make(chan struct{})
	e.stopping = stopping
	e.status.State = Stopping
	cmd, job, cancel, done := e.cmd, e.job, e.cancel, e.done
	ev := Event{Status: e.status}
	chans := s.subscribersLocked()
	s.mu.Unlock()
	s.broadcast(chans, ev)
	defer close(stopping)

	if cancel != nil {
		cancel()
	}
	var errs []error
	if cmd != nil {
		if err := stopProcessFn(cmd, job, stopTimeout); err != nil {
			errs = append(errs, fmt.Errorf("parar %s: %w", id, err))
		}
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(stopTimeout):
			errs = append(errs, fmt.Errorf("parar %s: laço não encerrou em %s", id, stopTimeout))
		}
	}
	s.releaseProcess(e)
	// Liberado antes do Stopped: um Stop que chegue a partir daqui já não
	// acha cmd nem job e só confirma a parada.
	s.mu.Lock()
	e.stopping = nil
	s.mu.Unlock()
	// C18.3: mesmo com erro o estado vira Stopped — o job foi fechado, então
	// o que sobrou da árvore já morreu.
	s.setState(e, Stopped, nil)
	return errors.Join(errs...)
}

// Restart para e inicia de novo. O Start acontece mesmo se o Stop reclamar:
// o estado já é Stopped e o job foi fechado.
func (s *Supervisor) Restart(id string) error {
	stopErr := s.Stop(id)
	startErr := s.Start(id)
	return errors.Join(stopErr, startErr)
}

// StartAll sobe todos na ordem de Add. É convergência, não comando: serviço
// que já está no ar não é erro — o botão "Iniciar tudo" clicado duas vezes,
// ou com parte da stack no ar, deve terminar com tudo rodando e sem alarme.
// Serviço sendo removido também não: ele está saindo do estado desejado.
func (s *Supervisor) StartAll() error {
	var errs []error
	for _, id := range s.ids() {
		if err := s.Start(id); err != nil && !errors.Is(err, ErrNotStopped) && !errors.Is(err, ErrRemoving) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// StopAll para todos na ordem inversa de Add (dependentes antes das bases);
// Replace preserva a posição do serviço trocado.
func (s *Supervisor) StopAll() error {
	ids := s.ids()
	var errs []error
	for i := len(ids) - 1; i >= 0; i-- {
		if err := s.Stop(ids[i]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Status devolve o retrato atual de um serviço.
func (s *Supervisor) Status(id string) (Status, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		return Status{}, false
	}
	return e.status, true
}

// List devolve todos os status ordenados por Group e depois ID.
func (s *Supervisor) List() []Status {
	s.mu.Lock()
	out := make([]Status, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e.status)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Subscribe devolve um canal de eventos com buffer e a função que o
// desinscreve. O canal NÃO é fechado pelo cancel: os envios acontecem fora do
// lock (regra de C17), e fechar concorrentemente causaria envio em canal
// fechado. Quem cancela simplesmente para de ler.
func (s *Supervisor) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, eventBuffer)
	s.mu.Lock()
	id := s.nextSub
	s.nextSub++
	s.subs[id] = ch
	s.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.subs, id)
			s.mu.Unlock()
		})
	}
}

// Logs devolve o ring de logs do serviço.
func (s *Supervisor) Logs(id string) (*LogRing, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		return nil, false
	}
	return e.ring, true
}

// Close para tudo e fecha os jobs POR SERVIÇO. O job global nunca é fechado:
// o próprio hyphp.exe está dentro dele e fechar o último handle mataria o
// processo. O kernel fecha esse handle no fim do processo e aí mata as
// sobras. Idempotente.
func (s *Supervisor) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	return s.StopAll()
}

// --- internos -------------------------------------------------------------

func (s *Supervisor) ids() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.order))
	copy(out, s.order)
	return out
}

func (s *Supervisor) subscribersLocked() []chan Event {
	out := make([]chan Event, 0, len(s.subs))
	for _, ch := range s.subs {
		out = append(out, ch)
	}
	return out
}

func (s *Supervisor) broadcast(chans []chan Event, ev Event) {
	for _, ch := range chans {
		select {
		case ch <- ev:
		default: // assinante lento perde a transição; o estado atual está em List()
		}
	}
}

// setState atualiza o estado e publica o evento. NUNCA pode ser chamado com
// s.mu preso.
func (s *Supervisor) setState(e *entry, st State, cause error) {
	s.mu.Lock()
	msg := e.status.LastError
	switch {
	case cause != nil:
		msg = cause.Error()
	case st == Ready || st == Stopped:
		msg = ""
	}
	if e.status.State == st && e.status.LastError == msg {
		s.mu.Unlock()
		return // em regime o probe roda a cada 5s: sem isto, evento a cada 5s
	}
	e.status.State = st
	e.status.LastError = msg
	if st == Stopped || st == Failed {
		e.status.PID = 0
	}
	ev := Event{Status: e.status}
	chans := s.subscribersLocked()
	s.mu.Unlock()
	s.logger.Info("serviço mudou de estado", "id", ev.Status.ID, "state", string(st), "erro", msg)
	s.broadcast(chans, ev)
}

// launch abre o arquivo de log (se houver) e cria o processo.
func (s *Supervisor) launch(e *entry) error {
	spec := e.spec
	var out io.Writer = e.ring
	var f *os.File
	if spec.LogPath != "" {
		if err := os.MkdirAll(filepath.Dir(spec.LogPath), 0o755); err != nil {
			s.logger.Warn("criar diretório de log", "id", spec.ID, "path", spec.LogPath, "err", err)
		} else if opened, err := os.OpenFile(spec.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err != nil {
			s.logger.Warn("abrir arquivo de log", "id", spec.ID, "path", spec.LogPath, "err", err)
		} else {
			f = opened
			out = io.MultiWriter(e.ring, f)
		}
	}
	cmd, job, err := startProcess(spec, out)
	if err != nil {
		if f != nil {
			f.Close()
		}
		return err
	}
	s.mu.Lock()
	e.cmd, e.job, e.logFile = cmd, job, f
	e.status.PID = cmd.Process.Pid
	e.status.StartedAt = time.Now()
	s.mu.Unlock()
	return nil
}

// stopProcessFn é variável para o teste contar e segurar as paradas.
var stopProcessFn = stopProcess

// releaseProcess fecha o job do serviço (matando o que sobrou da árvore), o
// arquivo de log e a última linha parcial do ring.
func (s *Supervisor) releaseProcess(e *entry) {
	s.mu.Lock()
	job, f := s.takeProcessLocked(e)
	s.mu.Unlock()
	closeProcess(e, job, f)
}

// takeProcessLocked tira cmd, job e log da entry. Separado de closeProcess
// para o laço de restart poder tirá-los na mesma seção crítica em que confere
// que nenhum Stop está em curso.
func (s *Supervisor) takeProcessLocked(e *entry) (windows.Handle, *os.File) {
	job, f := e.job, e.logFile
	e.cmd, e.job, e.logFile = nil, 0, nil
	e.status.PID = 0
	return job, f
}

func closeProcess(e *entry, job windows.Handle, f *os.File) {
	if job != 0 {
		windows.CloseHandle(job)
	}
	if f != nil {
		f.Close()
	}
	e.ring.flush()
}

// run acompanha um serviço: readiness, supervisão em regime e reinício.
// Sai quando o ctx é cancelado (Stop/Close cuidam do estado final) ou quando
// o serviço chega a Failed.
func (s *Supervisor) run(e *entry, ctx context.Context) {
	defer close(e.done)
	for {
		s.mu.Lock()
		cmd := e.cmd
		s.mu.Unlock()
		exited := make(chan error, 1)
		go func(c *exec.Cmd) { exited <- c.Wait() }(cmd)

		reason := s.supervise(e, ctx, exited)
		if ctx.Err() != nil {
			return
		}

		s.mu.Lock()
		if e.status.State == Stopping {
			// Corrida: o processo saiu sozinho no exato instante do Stop.
			// Quem chamou Stop é dono do estado final.
			s.mu.Unlock()
			return
		}
		pol := e.spec.Restart
		attempt := e.restarts + 1
		allowed := pol.Enabled && (pol.MaxRetries == 0 || attempt <= pol.MaxRetries)
		if allowed {
			e.restarts = attempt
			e.status.Restarts = attempt
		}
		// Tirados sob o mesmo lock da checagem de Stopping: soltar o lock antes
		// deixava um Stop capturar o job que este laço fecharia em seguida.
		job, f := s.takeProcessLocked(e)
		s.mu.Unlock()

		closeProcess(e, job, f)
		if !allowed {
			s.setState(e, Failed, reason)
			return
		}
		delay := nextDelay(pol, attempt)
		s.logger.Warn("reiniciando serviço", "id", e.spec.ID, "tentativa", attempt, "espera", delay.String(), "causa", reason)
		s.setState(e, Starting, reason)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
		if err := s.launch(e); err != nil {
			s.setState(e, Failed, fmt.Errorf("reiniciar %s: %w", e.spec.ID, err))
			return
		}
	}
}

// supervise leva o serviço de Starting a Ready e o acompanha até o processo
// sair. Devolve a causa da saída (ctx.Err() quando o Stop mandou parar).
func (s *Supervisor) supervise(e *entry, ctx context.Context, exited <-chan error) error {
	spec := e.spec
	deadline := time.Now().Add(spec.ProbeTimeout)
	timeout := time.NewTimer(spec.ProbeTimeout)
	defer timeout.Stop()
	tick := time.NewTicker(spec.ProbeInterval)
	defer tick.Stop()

	lastProbeErr := errors.New("nenhum probe concluiu")
	for ready := false; !ready; {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-exited:
			return exitReason(err)
		case <-timeout.C:
			// Processo vivo mas nunca ficou pronto: mata e trata como saída
			// inesperada (C18.3).
			s.killAndDrain(e, exited)
			return fmt.Errorf("probe não passou em %s: %w", spec.ProbeTimeout, lastProbeErr)
		case <-tick.C:
		}
		// O probe herda o prazo de readiness: um probe que bloqueia (AliveProbe
		// com Grace longo) não pode furar o ProbeTimeout.
		pctx, cancel := context.WithDeadline(ctx, deadline)
		err := spec.Probe.Check(pctx)
		cancel()
		if err == nil {
			ready = true
			break
		}
		lastProbeErr = err
	}

	s.setState(e, Ready, nil)
	rtick := time.NewTicker(readyProbeInterval)
	defer rtick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-exited:
			return exitReason(err)
		case <-rtick.C:
		}
		pctx, cancel := context.WithTimeout(ctx, readyProbeTimeout)
		err := spec.Probe.Check(pctx)
		cancel()
		if err != nil {
			s.setState(e, Degraded, err)
		} else {
			s.setState(e, Ready, nil)
		}
	}
}

// killAndDrain mata a árvore e espera o cmd.Wait() da goroutine de saída,
// para o laço não seguir com um processo meio morto.
func (s *Supervisor) killAndDrain(e *entry, exited <-chan error) {
	s.mu.Lock()
	cmd, job := e.cmd, e.job
	s.mu.Unlock()
	if cmd == nil {
		return
	}
	if err := stopProcessFn(cmd, job, stopTimeout); err != nil {
		s.logger.Warn("matar serviço travado", "id", e.spec.ID, "err", err)
	}
	select {
	case <-exited:
	case <-time.After(stopTimeout):
		s.logger.Warn("cmd.Wait não retornou após matar o job", "id", e.spec.ID)
	}
}
