// Package services contém os services Wails (bindings Go→TS), um por área da UI.
package services

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"

	"golang.org/x/sys/windows/registry"

	"hyphp/internal/i18n"
	"hyphp/internal/paths"
	"hyphp/internal/runtime"
	"hyphp/internal/state"
	"hyphp/internal/version"
)

// AppDeps são as dependências injetadas por main.go.
type AppDeps struct {
	Quit      func()       // marca "saindo" e chama app.Quit()
	State     *state.State // estado carregado em main.go (Editor, Terminal)
	StatePath string
	Logger    *slog.Logger
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

// shellOpen delega ao shell do Windows, que é o que acontece quando o usuário
// dá duplo clique: resolve o handler associado e abre na sessão interativa.
//
// A implementação anterior lançava `explorer.exe <path>` como processo filho.
// O HyPHP roda sem console e dentro de um Job Object kill-on-close, e o filho
// herda os dois — além de `SW_HIDE`, que o próprio processo herdeiro pode
// respeitar. ShellExecute não cria filho nosso: pede ao shell que abra, e o
// erro volta como código em vez de silêncio.
func shellOpen(target string) error {
	verb, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return fmt.Errorf("app: verbo invalido: %w", err)
	}
	// O shell exige separador nativo; um caminho com "/" abre a pasta errada
	// ou não abre nada.
	file, err := syscall.UTF16PtrFromString(filepath.FromSlash(target))
	if err != nil {
		return fmt.Errorf("app: caminho invalido %q: %w", target, err)
	}
	if err := windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("app: abrir %q: %w", target, err)
	}
	return nil
}

// OpenInEditor abre o caminho no editor configurado (state.Editor) ou no VS Code (`code`).
func (a *AppService) OpenInEditor(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("app: caminho inexistente: %w", err)
	}
	if editor := a.d.State.Editor; editor != "" {
		return a.logged("abrir no editor", path, startHidden(editor, path))
	}
	// `code` é code.cmd no PATH; passa pelo cmd.exe para resolver o .cmd.
	return a.logged("abrir no editor", path, startHidden("cmd.exe", "/c", "code", path))
}

// isWindowsTerminal responde se o executável é o Windows Terminal, que precisa
// de tratamento próprio: ele ignora o diretório de trabalho herdado e abre o
// perfil na pasta configurada nele, então sem "-d" o terminal abre no lugar
// errado — que é indistinguível de "o botão não funciona".
func isWindowsTerminal(exe string) bool {
	return strings.EqualFold(filepath.Base(exe), "wt.exe")
}

// OpenTerminal abre um terminal no diretório: state.Terminal, senão Windows Terminal, senão cmd.exe.
func (a *AppService) OpenTerminal(path string) error {
	if err := mustDir(path); err != nil {
		return err
	}
	if term := a.d.State.Terminal; term != "" {
		if isWindowsTerminal(term) {
			return a.logged("abrir terminal", path, start(exec.Command(term, "-d", path)))
		}
		cmd := exec.Command(term)
		cmd.Dir = path
		return a.logged("abrir terminal", path, start(cmd))
	}
	if wt, err := exec.LookPath("wt.exe"); err == nil {
		return a.logged("abrir terminal", path, start(exec.Command(wt, "-d", path)))
	}
	// `start` abre uma nova janela de console herdando o cwd do cmd /c (= path).
	cmd := exec.Command("cmd.exe", "/c", "start", "", "cmd.exe")
	cmd.Dir = path
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return a.logged("abrir terminal", path, start(cmd))
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

// pathComPHP devolve o PATH com dir à frente e se houve mudança. Comparação
// sem diferenciar maiúsculas porque caminho no Windows é case-insensitive:
// comparar byte a byte reinseriria o mesmo diretório a cada chamada.
func pathComPHP(atual, dir string) (string, bool) {
	for _, p := range strings.Split(atual, ";") {
		if strings.EqualFold(strings.TrimRight(strings.TrimSpace(p), `\`), strings.TrimRight(dir, `\`)) {
			return atual, false
		}
	}
	if strings.TrimSpace(atual) == "" {
		return dir, true
	}
	return dir + ";" + atual, true
}

// AddDefaultPHPToUserPath põe a série padrão de PHP no PATH do usuário, para
// `php` e `composer` no terminal usarem a mesma versão que o HyPHP serve.
//
// Escreve em HKCU\Environment: a Path da máquina exigiria UAC, e o produto só
// eleva em ApplyHosts e InstallCA (spec §11).
func (a *AppService) AddDefaultPHPToUserPath() error {
	inst, ok := a.d.DefaultPHP()
	if !ok {
		return errors.New(i18n.T("err.app.noPHP"))
	}

	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("abrir HKCU\\Environment: %w", err)
	}
	defer k.Close()

	atual, _, err := k.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("ler Path do usuário: %w", err)
	}
	novo, mudou := pathComPHP(atual, inst.Dir)
	if !mudou {
		return nil
	}
	// EXPAND_SZ preserva referências como %USERPROFILE% que já estejam no
	// valor; gravar como SZ as congelaria em texto literal.
	if err := k.SetExpandStringValue("Path", novo); err != nil {
		return fmt.Errorf("gravar Path do usuário: %w", err)
	}
	return nil
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
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
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
