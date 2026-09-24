// Package elevate executa o hyphp-helper.exe sob UAC e espera o resultado.
// É o único lugar do produto que pede elevação (spec §11).
package elevate

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"hyphp/internal/paths"
)

// helperName é o nome do binário elevado, sempre ao lado do executável em curso.
const helperName = "hyphp-helper.exe"

// helperTimeoutMS é quanto esperamos o helper terminar. 2 minutos porque `mkcert -install`
// abre o diálogo "Aviso de Segurança" do Windows e fica parado esperando o usuário clicar.
const helperTimeoutMS = 120000

// Máscaras de SHELLEXECUTEINFOW (shellapi.h).
const (
	SEE_MASK_NOCLOSEPROCESS = 0x00000040 // preenche hProcess: é o que permite esperar e ler o exit code
	SEE_MASK_NOASYNC        = 0x00000100 // obrigatório fora de thread com bomba de mensagens
	SEE_MASK_FLAG_NO_UI     = 0x00000400 // sem caixas de erro do shell (o UAC continua aparecendo)
)

// ERROR_CANCELLED é o que o shell devolve quando o usuário clica "Não" no UAC.
const ERROR_CANCELLED = 1223

var (
	// ErrElevationDenied: o usuário recusou o UAC. Vira o warning "elevation-denied" no Stack.
	ErrElevationDenied = errors.New("elevação negada pelo usuário (UAC)")
	// ErrHelperFailed: o helper rodou e terminou com código ≠ 0. Sentinela de HelperError.
	ErrHelperFailed = errors.New("hyphp-helper falhou")
	// ErrHelperTimeout: o helper não terminou em helperTimeoutMS.
	ErrHelperTimeout = errors.New("hyphp-helper não terminou a tempo")
)

// HelperError descreve uma execução do helper que terminou com código ≠ 0.
// Message vem do JSON que o helper grava no arquivo passado em --result.
type HelperError struct {
	ExitCode int
	Message  string
}

func (e *HelperError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("hyphp-helper terminou com código %d", e.ExitCode)
	}
	return fmt.Sprintf("hyphp-helper terminou com código %d: %s", e.ExitCode, e.Message)
}

// Unwrap liga HelperError à sentinela: errors.Is(err, ErrHelperFailed) funciona,
// e quem precisa do código usa errors.As(err, &he).
func (e *HelperError) Unwrap() error { return ErrHelperFailed }

// SHELLEXECUTEINFO espelha SHELLEXECUTEINFOW campo a campo, na ordem de shellapi.h.
// x/sys/windows v0.48.0 expõe ShellExecute (sem handle de processo) mas não ShellExecuteEx,
// então a struct e a chamada moram aqui. Tamanho em amd64: 112 bytes.
type SHELLEXECUTEINFO struct {
	cbSize         uint32
	fMask          uint32
	hwnd           windows.Handle
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       windows.Handle
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      windows.Handle
	dwHotKey       uint32
	hIconOrMonitor windows.Handle
	hProcess       windows.Handle
}

var (
	shell32             = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteExW = shell32.NewProc("ShellExecuteExW")
)

// RunElevated roda exe com args sob UAC e espera o término.
// Antepõe sempre `--result <run>/helper-<16 hex>.json` aos args: é por esse arquivo que a
// mensagem de erro do helper volta (o stdout de um processo elevado com SW_HIDE se perde).
// O arquivo é apagado ao final, dê certo ou errado.
//
// Erros: ErrElevationDenied (UAC recusado), ErrHelperTimeout (passou de 2 min),
// *HelperError (código ≠ 0, com a mensagem do helper).
func RunElevated(exe string, args []string) error {
	resultPath, err := newResultPath()
	if err != nil {
		return err
	}
	defer os.Remove(resultPath)

	full := make([]string, 0, len(args)+2)
	full = append(full, "--result", resultPath)
	full = append(full, args...)

	code, err := shellExecuteWait("runas", exe, quoteArgs(full))
	if err != nil {
		return err
	}
	if code != 0 {
		return &HelperError{ExitCode: int(code), Message: readResultMessage(resultPath)}
	}
	return nil
}

// HelperPath devolve o caminho do hyphp-helper.exe ao lado do executável em curso.
// Erro quando o arquivo não existe — é o caso de dev sem `wails3 task build:helper`.
func HelperPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("HelperPath: %w", err)
	}
	return helperPathIn(filepath.Dir(exe))
}

func helperPathIn(dir string) (string, error) {
	p := filepath.Join(dir, helperName)
	st, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("HelperPath: %s ausente (rode `wails3 task build:helper`): %w", p, err)
	}
	if st.IsDir() {
		return "", fmt.Errorf("HelperPath: %s é um diretório", p)
	}
	return p, nil
}

// quoteArgs junta os argumentos numa única linha de parâmetros no formato que
// CommandLineToArgvW (usado pelo runtime do processo filho) sabe desfazer.
func quoteArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = syscall.EscapeArg(a)
	}
	return strings.Join(parts, " ")
}

// newResultPath sorteia <paths.Run()>/helper-<16 hex>.json e garante o diretório.
func newResultPath() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("RunElevated: sortear nome do arquivo de resultado: %w", err)
	}
	dir := paths.Run()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("RunElevated: criar %s: %w", dir, err)
	}
	return filepath.Join(dir, "helper-"+hex.EncodeToString(buf[:])+".json"), nil
}

// readResultMessage lê o JSON do helper. Arquivo ausente ou ilegível → mensagem vazia:
// o exit code sozinho já identifica a falha.
func readResultMessage(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return res.Error
}

// shellExecuteWait chama ShellExecuteExW, espera o processo e devolve o exit code.
// verb é "runas" em produção; os testes usam "open" para exercitar o caminho sem UAC.
func shellExecuteWait(verb, exe, params string) (uint32, error) {
	verbPtr, err := windows.UTF16PtrFromString(verb)
	if err != nil {
		return 0, fmt.Errorf("verbo %q: %w", verb, err)
	}
	filePtr, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, fmt.Errorf("exe %q: %w", exe, err)
	}
	dirPtr, err := windows.UTF16PtrFromString(filepath.Dir(exe))
	if err != nil {
		return 0, fmt.Errorf("dir de %q: %w", exe, err)
	}
	var paramsPtr *uint16
	if params != "" {
		paramsPtr, err = windows.UTF16PtrFromString(params)
		if err != nil {
			return 0, fmt.Errorf("parâmetros %q: %w", params, err)
		}
	}

	info := SHELLEXECUTEINFO{
		fMask:        SEE_MASK_NOCLOSEPROCESS | SEE_MASK_NOASYNC | SEE_MASK_FLAG_NO_UI,
		lpVerb:       verbPtr,
		lpFile:       filePtr,
		lpParameters: paramsPtr,
		lpDirectory:  dirPtr,
		nShow:        windows.SW_HIDE,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))

	ret, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		if errno, ok := callErr.(syscall.Errno); ok && errno == ERROR_CANCELLED {
			return 0, ErrElevationDenied
		}
		return 0, fmt.Errorf("ShellExecuteExW %s: %w", exe, callErr)
	}
	if info.hProcess == 0 {
		return 0, fmt.Errorf("ShellExecuteExW %s: sem handle de processo (SEE_MASK_NOCLOSEPROCESS ignorado)", exe)
	}
	defer windows.CloseHandle(info.hProcess)

	event, err := windows.WaitForSingleObject(info.hProcess, helperTimeoutMS)
	if err != nil {
		return 0, fmt.Errorf("WaitForSingleObject: %w", err)
	}
	switch event {
	case windows.WAIT_OBJECT_0:
	case uint32(windows.WAIT_TIMEOUT): // WAIT_TIMEOUT é syscall.Errno em x/sys/windows
		return 0, fmt.Errorf("%w (%d s)", ErrHelperTimeout, helperTimeoutMS/1000)
	default:
		return 0, fmt.Errorf("WaitForSingleObject devolveu 0x%x", event)
	}

	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return 0, fmt.Errorf("GetExitCodeProcess: %w", err)
	}
	return code, nil
}
