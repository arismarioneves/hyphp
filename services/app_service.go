// Package services contém os services Wails (bindings Go→TS), um por área da UI.
package services

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	goruntime "runtime"

	"hyphp/internal/credits"
	"hyphp/internal/i18n"
	"hyphp/internal/paths"
	"hyphp/internal/project"
	"hyphp/internal/runtime"
	"hyphp/internal/state"
	"hyphp/internal/sysinfo"
	"hyphp/internal/update"
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
	// Tray e Dark dizem se a sessão tem bandeja e se o tema do sistema é o
	// escuro. Mudam com o app aberto, e main.go avisa no platform:changed.
	Tray func() bool
	Dark func() bool
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

// Platform é o sistema em que o app roda e o que a sessão gráfica oferece. A
// interface escolhe os textos por ele e o relê no evento platform:changed.
type Platform struct {
	OS string `json:"os"` // runtime.GOOS: "windows", "darwin", "linux"
	// Tray: com bandeja, fechar esconde a janela; sem ela (GNOME sem a
	// extensão AppIndicator), minimiza.
	Tray bool `json:"tray"`
	// Dark é o tema escuro do sistema. Só o Linux o usa: lá o WebView não o
	// segue no prefers-color-scheme.
	Dark bool `json:"dark"`
}

// Platform devolve o sistema, a bandeja e o tema de agora.
func (a *AppService) Platform() Platform {
	return Platform{OS: goruntime.GOOS, Tray: a.d.Tray(), Dark: a.d.Dark()}
}

// About são os dados da tela Sobre.
type About struct {
	Version      string   `json:"version"`
	OS           string   `json:"os"`   // "Windows 11 (10.0.26100)", "macOS 15.1"
	Arch         string   `json:"arch"` // "x64", "arm64"
	Root         string   `json:"root"`
	Site         string   `json:"site"`
	Repository   string   `json:"repository"`
	ReleaseNotes string   `json:"releaseNotes"` // release da versão em uso
	NewIssue     string   `json:"newIssue"`
	License      string   `json:"license"`
	Copyright    string   `json:"copyright"`
	Author       string   `json:"author"`
	AuthorLogin  string   `json:"authorLogin"`
	Contributors []string `json:"contributors"` // logins do GitHub, de quem mais contribuiu para quem menos
	Support      string   `json:"support"`
}

// About reúne versão, sistema, links e créditos para a tela Sobre.
func (a *AppService) About() (About, error) {
	contributors, err := credits.Contributors()
	if err != nil {
		return About{}, err
	}
	repo := "https://github.com/" + update.GitHubRepo
	return About{
		Version:      version.Current,
		OS:           sysinfo.OS(),
		Arch:         sysinfo.Arch(),
		Root:         paths.Root(),
		Site:         project.SiteURL,
		Repository:   repo,
		ReleaseNotes: repo + "/releases/tag/v" + version.Current,
		NewIssue:     repo + "/issues/new",
		License:      credits.License,
		Copyright:    credits.Copyright,
		Author:       credits.Author,
		AuthorLogin:  credits.AuthorLogin,
		Contributors: contributors,
		Support:      credits.SupportURL,
	}, nil
}

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
