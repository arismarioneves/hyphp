package services

import (
	"context"
	"sync"
	"time"

	"hyphp/internal/autostart"
	"hyphp/internal/i18n"
	"hyphp/internal/stack"
	"hyphp/internal/state"
)

// SettingsService lê e grava state.json via Stack e dispara Reconcile quando
// algo que afeta processos/configs mudou.
type SettingsService struct {
	mu   sync.Mutex
	stk  *stack.Stack
	emit func(name string, data any)
	// onLanguage roda depois que o idioma muda, para o que o Go desenha fora
	// da UI (o menu do tray) trocar de texto. Nil em testes.
	onLanguage func()
	// applyAutostart é autostart.Apply; teste troca para não mexer na chave
	// Run real do usuário (Set com Autostart=false apagaria a entrada HyPHP).
	applyAutostart func(enabled bool) error
}

func NewSettingsService(stk *stack.Stack, emit func(name string, data any), onLanguage func()) *SettingsService {
	return &SettingsService{stk: stk, emit: emit, onLanguage: onLanguage, applyAutostart: autostart.Apply}
}

// SystemLanguage devolve o idioma que vale quando state.Language está vazio.
// A UI pergunta ao Go, e não ao navigator do WebView, para os dois lados
// escolherem igual.
func (s *SettingsService) SystemLanguage() string {
	return string(i18n.Resolve(""))
}

// Languages lista os idiomas oferecidos, na ordem da interface.
func (s *SettingsService) Languages() []string {
	out := make([]string, len(i18n.Supported))
	for i, l := range i18n.Supported {
		out[i] = string(l)
	}
	return out
}

func (s *SettingsService) Get() state.State {
	return s.stk.State()
}

func (s *SettingsService) Warnings() []stack.Warning {
	return s.stk.Warnings()
}

// ApplyHosts grava os domínios dos projetos no arquivo hosts do Windows.
// É a única ação do produto que pede UAC por causa de domínios, e existe como
// ação explícita justamente para o app não exigir privilégio só para abrir:
// o Reconcile apenas reporta o warning "hosts-pending", e a UI oferece o botão.
func (s *SettingsService) ApplyHosts() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := s.stk.ApplyHosts(ctx); err != nil {
		return err
	}
	s.emit("stack:warnings", s.stk.Warnings())
	return nil
}

// InstallCA instala o certificado raiz local, habilitando HTTPS nos sites.
// Pede UAC, por isso é ação explícita: o Reconcile apenas reporta "ca-pending".
func (s *SettingsService) InstallCA() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := s.stk.InstallCA(ctx); err != nil {
		return err
	}
	s.emit("stack:warnings", s.stk.Warnings())
	return nil
}

// ApplyWildcardDNS registra a regra de DNS que faz *.dominio.test resolver.
// Pede UAC, por isso é ação explícita: o Reconcile apenas reporta
// "wildcard-pending".
func (s *SettingsService) ApplyWildcardDNS() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := s.stk.ApplyWildcardDNS(ctx); err != nil {
		return err
	}
	s.emit("stack:warnings", s.stk.Warnings())
	return nil
}

// RemoveWildcardDNS desfaz a regra. Sem esta porta, quem parasse de usar
// wildcard ficaria com o namespace .test apontando para um resolvedor morto.
func (s *SettingsService) RemoveWildcardDNS() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := s.stk.RemoveWildcardDNS(ctx); err != nil {
		return err
	}
	s.emit("stack:warnings", s.stk.Warnings())
	return nil
}

// Set aplica os campos da tela Configurações. SchemaVersion e PortAlloc
// pertencem ao Stack e são ignorados. Roots (ProjectsService),
// PHPExtensions (RuntimesService), WebServer (SwitchWebServer, chamado
// pelos cards da tela e por `hyphp web`) e DBEngine (SwitchDatabase) também
// ficam de fora: são mudados por outros caminhos, e a cópia que a UI manda
// pode ser anterior à mudança — gravá-la a desfaria (uma pasta recém-adicionada
// sumia ao recolher a sidebar; um salvar qualquer trocava o web server de
// volta). DefaultPHP fica: a tela tem o seletor "Versão padrão", e o rascunho
// dela recebe o valor novo quando o Runtimes ou a CLI o trocam
// (settings:changed).
func (s *SettingsService) Set(in state.State) error {
	if err := validateSettings(in); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	cur := s.stk.State()
	relevant := cur.DefaultPHP != in.DefaultPHP || cur.PoolSize != in.PoolSize ||
		cur.HTTPPort != in.HTTPPort || cur.HTTPSPort != in.HTTPSPort ||
		cur.MySQLPort != in.MySQLPort || cur.MailpitSMTPPort != in.MailpitSMTPPort ||
		cur.MailpitHTTPPort != in.MailpitHTTPPort ||
		// Avisos e a página padrão do web server são escritos pelo Reconcile
		// no idioma atual: trocar o idioma tem de reescrevê-los.
		cur.Language != in.Language

	// O toggle "Iniciar o HyPHP no login" só valia como campo persistido; sem
	// isto a tela promete um comportamento que não acontece. Aplicar antes de
	// gravar mantém state.json honesto: se o registro recusar, nada é
	// persistido e a UI não passa a exibir um estado que a máquina não tem.
	if err := s.applyAutostart(in.Autostart); err != nil {
		return err
	}

	err := s.stk.UpdateState(func(st *state.State) {
		st.DefaultPHP = in.DefaultPHP
		st.PoolSize = in.PoolSize
		st.HTTPPort = in.HTTPPort
		st.HTTPSPort = in.HTTPSPort
		st.MySQLPort = in.MySQLPort
		st.MailpitSMTPPort = in.MailpitSMTPPort
		st.MailpitHTTPPort = in.MailpitHTTPPort
		st.Editor = in.Editor
		st.Terminal = in.Terminal
		st.SidebarCollapsed = in.SidebarCollapsed
		st.Autostart = in.Autostart
		st.AutoUpdateOff = in.AutoUpdateOff
		st.Theme = in.Theme
		st.Language = in.Language
	})
	if err != nil {
		return err
	}
	if cur.Language != in.Language {
		i18n.SetCurrent(i18n.Resolve(in.Language))
		if s.onLanguage != nil {
			s.onLanguage()
		}
	}

	if relevant {
		ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
		defer cancel()
		if _, err := s.stk.Reconcile(ctx); err != nil {
			s.emit("settings:changed", s.stk.State())
			return i18n.Errorf("err.reconcile", err)
		}
	}
	s.emit("settings:changed", s.stk.State())
	return nil
}

// SetSidebarCollapsed grava só o estado da sidebar. O toggle não pode passar
// por Set: a cópia da UI levaria junto campos que outros serviços mudaram.
func (s *SettingsService) SetSidebarCollapsed(collapsed bool) error {
	if err := s.stk.UpdateState(func(st *state.State) { st.SidebarCollapsed = collapsed }); err != nil {
		return err
	}
	s.emit("settings:changed", s.stk.State())
	return nil
}

func (s *SettingsService) SwitchWebServer(name string) error {
	ws := state.WebServerName(name)
	if ws != state.Apache && ws != state.Nginx {
		return i18n.Errorf("err.settings.webServer", name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()
	if err := s.stk.SwitchWebServer(ctx, ws); err != nil {
		return err
	}
	s.emit("settings:changed", s.stk.State())
	return nil
}

// dbSwitchTimeout cobre, no pior caso, o Reconcile com a primeira
// inicialização do datadir do motor novo, a espera de ele ficar pronto e o
// Reconcile de volta se não ficar.
const dbSwitchTimeout = 3 * time.Minute

// SwitchDatabase troca o motor de banco (mysql|mariadb). Imediata, como a
// troca de web server: o banco atual para e o outro sobe na mesma porta; se
// o novo não ficar pronto, o anterior volta.
func (s *SettingsService) SwitchDatabase(engine string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), dbSwitchTimeout)
	defer cancel()
	err := s.stk.SwitchDatabase(ctx, engine)
	s.emit("settings:changed", s.stk.State())
	s.emit("stack:warnings", s.stk.Warnings())
	return err
}

func validateSettings(st state.State) error {
	switch st.Theme {
	case "", state.ThemeDark, state.ThemeLight, state.ThemeSystem:
	default:
		return i18n.Errorf("err.settings.theme", st.Theme)
	}
	if !i18n.Valid(st.Language) {
		return i18n.Errorf("err.settings.language", st.Language)
	}
	if st.PoolSize < 1 || st.PoolSize > 16 {
		return i18n.Errorf("err.settings.poolSize", st.PoolSize)
	}
	if st.DefaultPHP != "" && !phpMajorRe.MatchString(st.DefaultPHP) {
		return i18n.Errorf("err.settings.defaultPhp", st.DefaultPHP)
	}
	ports := map[string]int{
		"httpPort": st.HTTPPort, "httpsPort": st.HTTPSPort, "mysqlPort": st.MySQLPort,
		"mailpitSmtpPort": st.MailpitSMTPPort, "mailpitHttpPort": st.MailpitHTTPPort,
	}
	seen := map[int]string{}
	for name, port := range ports {
		if port < 1 || port > 65535 {
			return i18n.Errorf("err.settings.portRange", name, port)
		}
		if other, dup := seen[port]; dup {
			return i18n.Errorf("err.settings.portDup", other, name, port)
		}
		seen[port] = name
	}
	return nil
}
