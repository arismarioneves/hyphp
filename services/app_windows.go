package services

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"golang.org/x/sys/windows/registry"

	"hyphp/internal/runtime"
	"hyphp/internal/sysproc"
)

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

// openInEditor abre o caminho no editor configurado (state.Editor) ou no VS Code (`code`).
func (a *AppService) openInEditor(path string) error {
	if editor := expandEnv(a.d.State().Editor); editor != "" {
		return startHidden(editor, path)
	}
	// `code` é code.cmd no PATH; passa pelo cmd.exe para resolver o .cmd. A
	// linha é montada à mão porque o os/exec só põe aspas quando há espaço, e
	// o cmd interpretaria `&`, `^` ou `(` de um caminho como sintaxe.
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: editorCmdLine(path)}
	sysproc.Hide(cmd)
	return start(cmd)
}

// editorCmdLine monta a linha do cmd.exe que abre path no VS Code. Entre
// aspas o cmd trata `&`, `^`, `(` e `)` como texto; aspas não existem em
// caminho do Windows. A única barra que precisa dobrar é a final (`C:\`):
// quem lê os argumentos do code.cmd tomaria `\"` por aspas escapadas.
func editorCmdLine(path string) string {
	if strings.HasSuffix(path, `\`) {
		path += `\`
	}
	return `cmd.exe /c code "` + path + `"`
}

// expandEnv expande %VAR% do caminho do editor/terminal: é assim que o
// Windows mostra o destino dos atalhos (%LOCALAPPDATA%\Programs\...), e o
// os/exec não expande nada. Variável inexistente fica como está (regra do
// Windows); se a expansão falhar, o valor original segue e o erro do start
// mostra o que foi tentado.
func expandEnv(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	if x, err := registry.ExpandString(s); err == nil {
		return x
	}
	return s
}

// isWindowsTerminal responde se o executável é o Windows Terminal, que precisa
// de tratamento próprio: ele ignora o diretório de trabalho herdado e abre o
// perfil na pasta configurada nele, então sem "-d" o terminal abre no lugar
// errado — que é indistinguível de "o botão não funciona".
func isWindowsTerminal(exe string) bool {
	return strings.EqualFold(filepath.Base(exe), "wt.exe")
}

// openTerminal abre um terminal no diretório: state.Terminal, senão Windows Terminal, senão cmd.exe.
func (a *AppService) openTerminal(path string) error {
	if term := expandEnv(a.d.State().Terminal); term != "" {
		if isWindowsTerminal(term) {
			return start(exec.Command(term, "-d", path))
		}
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
	sysproc.Hide(cmd)
	return start(cmd)
}

// pathComDir devolve o PATH com dir à frente e se houve mudança. Comparação
// sem diferenciar maiúsculas porque caminho no Windows é case-insensitive:
// comparar byte a byte reinseriria o mesmo diretório a cada chamada.
func pathComDir(atual, dir string) (string, bool) {
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

// readUserPath lê a Path de HKCU\Environment ("" se não existir).
func readUserPath() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("abrir HKCU\\Environment: %w", err)
	}
	defer k.Close()
	atual, _, err := k.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return "", fmt.Errorf("ler Path do usuário: %w", err)
	}
	return atual, nil
}

// addToUserPath põe dir no início da Path do usuário, se ainda não estiver.
//
// Escreve em HKCU\Environment: a Path da máquina exigiria UAC, e o produto só
// eleva em ApplyHosts e InstallCA (spec §11). Depois avisa o Windows
// (WM_SETTINGCHANGE "Environment"): sem isso o Explorer segue com o ambiente
// antigo, e um terminal aberto pelo menu Iniciar não acharia o diretório novo
// até o próximo login.
func addToUserPath(dir string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("abrir HKCU\\Environment: %w", err)
	}
	defer k.Close()

	atual, _, err := k.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("ler Path do usuário: %w", err)
	}
	novo, mudou := pathComDir(atual, dir)
	if !mudou {
		return nil
	}
	// EXPAND_SZ preserva referências como %USERPROFILE% que já estejam no
	// valor; gravar como SZ as congelaria em texto literal.
	if err := k.SetExpandStringValue("Path", novo); err != nil {
		return fmt.Errorf("gravar Path do usuário: %w", err)
	}
	broadcastEnvironment()
	return nil
}

// cliPathDir: no Windows a pasta da CLI (cli\ ao lado do hyphp.exe) é a que
// entra no PATH.
func cliPathDir(exe string) string { return filepath.Dir(exe) }

func addCLIToUserPath(exe string) error { return addToUserPath(filepath.Dir(exe)) }

func addPHPToUserPath(inst runtime.Installed) error { return addToUserPath(inst.Dir) }

// RefreshPathLinks: no Windows o PATH aponta direto para as pastas; não há
// atalhos a refazer.
func RefreshPathLinks(string, func() (runtime.Installed, bool)) error { return nil }

// shadowingPHP: no Windows o botão grava a pasta da série no PATH do usuário;
// a conferência do php que o shell acha é só do Mac.
func shadowingPHP() (string, error) { return "", nil }

var procSendMessageTimeout = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")

// broadcastEnvironment manda WM_SETTINGCHANGE "Environment" a todas as
// janelas de topo. SMTO_ABORTIFHUNG com 5 s: uma janela travada não prende o
// app. O resultado não importa: a Path já está gravada.
func broadcastEnvironment() {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
	)
	env, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	var res uintptr
	_, _, _ = procSendMessageTimeout.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, uintptr(unsafe.Pointer(&res)))
}

// startHidden inicia exe sem janela de console própria e não espera término.
func startHidden(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	sysproc.Hide(cmd)
	return start(cmd)
}
