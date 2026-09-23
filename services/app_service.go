// Package services contém os services Wails (bindings Go→TS), um por área da UI.
package services

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"syscall"

	"hyphp/internal/paths"
	"hyphp/internal/state"
)

// Version é a versão do HyPHP exibida em Configurações › Sobre.
const Version = "0.1.0"

// AppDeps são as dependências injetadas por main.go.
type AppDeps struct {
	Quit      func()       // marca "saindo" e chama app.Quit()
	State     *state.State // estado carregado em main.go (Editor, Terminal)
	StatePath string
	Logger    *slog.Logger
}

// AppService expõe ações globais do app à UI.
type AppService struct {
	d AppDeps
}

func NewAppService(d AppDeps) *AppService {
	return &AppService{d: d}
}

// Version retorna a versão do HyPHP.
func (a *AppService) Version() string { return Version }

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
	return startHidden("rundll32.exe", "url.dll,FileProtocolHandler", rawURL)
}

// OpenFolder abre o diretório no Explorer.
func (a *AppService) OpenFolder(path string) error {
	if err := mustDir(path); err != nil {
		return err
	}
	return startHidden("explorer.exe", path)
}

// OpenInEditor abre o caminho no editor configurado (state.Editor) ou no VS Code (`code`).
func (a *AppService) OpenInEditor(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("app: caminho inexistente: %w", err)
	}
	if editor := a.d.State.Editor; editor != "" {
		return startHidden(editor, path)
	}
	// `code` é code.cmd no PATH; passa pelo cmd.exe para resolver o .cmd.
	return startHidden("cmd.exe", "/c", "code", path)
}

// OpenTerminal abre um terminal no diretório: state.Terminal, senão Windows Terminal, senão cmd.exe.
func (a *AppService) OpenTerminal(path string) error {
	if err := mustDir(path); err != nil {
		return err
	}
	if term := a.d.State.Terminal; term != "" {
		cmd := exec.Command(term)
		cmd.Dir = path
		return start(cmd)
	}
	if wt, err := exec.LookPath("wt.exe"); err == nil {
		return start(exec.Command(wt, "-d", path))
	}
	// `start` abre uma nova janela de console herdando o cwd do cmd /c (= path).
	cmd := exec.Command("cmd.exe", "/c", "start", "", "cmd.exe")
	cmd.Dir = path
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return start(cmd)
}

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

// startHidden inicia exe sem janela de console própria e não espera término.
func startHidden(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return start(cmd)
}

// start dispara o processo e colhe o exit code em background (evita zumbis de handle).
func start(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("app: iniciar %s: %w", cmd.Path, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
