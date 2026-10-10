// Package services contém os services Wails (bindings Go→TS), um por área da UI.
package services

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"

	"hyphp/internal/i18n"
	"hyphp/internal/paths"
	"hyphp/internal/project"
	"hyphp/internal/runtime"
	"hyphp/internal/state"
	"hyphp/internal/version"
)

// AppDeps são as dependências injetadas por main.go.
type AppDeps struct {
	Quit   func()             // marca "saindo" e chama app.Quit()
	State  func() state.State // cópia sob o lock do Stack (Editor, Terminal)
	Logger *slog.Logger
	// DefaultPHP resolve a série padrão pela mesma regra do Stack
	// (state.DefaultPHP, vazio = maior instalada); ligado em main.go.
	DefaultPHP func() (runtime.Installed, bool)
}

// AppService expõe ações globais do app à UI.
type AppService struct {
	d AppDeps
}

func NewAppService(d AppDeps) *AppService {
	return &AppService{d: d}
}

// Version retorna a versão do HyPHP.
func (a *AppService) Version() string { return version.Current }

// SiteURL retorna a página do HyPHP, a mesma citada no cabeçalho do
// hyphp.yaml, para o Sobre de Configurações.
func (a *AppService) SiteURL() string { return project.SiteURL }

// RuntimeRoot retorna a raiz de runtime em uso (bin/, etc/, var/, log/).
func (a *AppService) RuntimeRoot() string { return paths.Root() }

// Quit encerra o app de verdade (não apenas oculta a janela).
func (a *AppService) Quit() { a.d.Quit() }

// OpenExternal abre uma URL http/https no browser padrão do Windows.
func (a *AppService) OpenExternal(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("app: url invalida: %q", rawURL)
	}
	return a.logged("abrir url", rawURL, shellOpen(rawURL))
}

// OpenFolder abre o diretório no Explorer.
func (a *AppService) OpenFolder(path string) error {
	if err := mustDir(path); err != nil {
		return err
	}
	return a.logged("abrir pasta", path, shellOpen(path))
}

// OpenInEditor abre o caminho no editor configurado (state.Editor) ou no
// editor padrão do SO; a forma de abrir fica em openInEditor, por SO.
func (a *AppService) OpenInEditor(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("app: caminho inexistente: %w", err)
	}
	return a.logged("abrir no editor", path, a.openInEditor(path))
}

// OpenTerminal abre um terminal no diretório: state.Terminal, senão o
// terminal padrão do SO (openTerminal, por SO).
func (a *AppService) OpenTerminal(path string) error {
	if err := mustDir(path); err != nil {
		return err
	}
	return a.logged("abrir terminal", path, a.openTerminal(path))
}

// logged registra a falha antes de devolvê-la. Estas ações terminam em
// programas externos, e quando uma delas não faz nada visível o log é a única
// forma de saber se o HyPHP tentou, com qual caminho, e o que o Windows disse.
func (a *AppService) logged(acao, path string, err error) error {
	if err != nil {
		a.d.Logger.Warn("app: "+acao, "path", path, "err", err)
	}
	return err
}

// AddDefaultPHPToUserPath põe a série padrão de PHP no PATH do usuário, para
// `php` e `composer` no terminal usarem a mesma versão que o HyPHP serve.
func (a *AppService) AddDefaultPHPToUserPath() error {
	inst, ok := a.d.DefaultPHP()
	if !ok {
		return errors.New(i18n.T("err.app.noPHP"))
	}
	return addPHPToUserPath(inst)
}

// ShadowingPHP devolve o php que o Terminal acha antes do PHP do HyPHP, ou ""
// quando o do HyPHP vence. No Mac, o php do Homebrew em /opt/homebrew/bin vem
// antes do /etc/paths.d no PATH, e sem o aviso o botão pareceria não funcionar.
func (a *AppService) ShadowingPHP() (string, error) { return shadowingPHP() }

func mustDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("app: diretorio inexistente: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("app: nao e diretorio: %s", path)
	}
	return nil
}

// start dispara o processo e colhe o exit code em background (evita zumbis de handle).
func start(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("app: iniciar %s: %w", cmd.Path, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
