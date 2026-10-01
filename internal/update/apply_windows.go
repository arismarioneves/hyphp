package update

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"hyphp/internal/elevate"
)

// ApplyFlag é o primeiro argumento do modo que aplica o update. main.go o
// trata antes de tudo: nesse modo não há log em paths.Log(), nem Wails, nem
// paths.Root() — a cópia roda de var/update/, e a raiz calculada a partir
// dela seria a pasta errada.
const ApplyFlag = "--aplicar-update"

const (
	// createBreakawayFromJob tira o atualizador do Job Object kill-on-close do
	// app (spec §7.1); sem isso o kernel o mataria junto com o app, que é
	// exatamente o momento em que ele precisa estar vivo.
	createBreakawayFromJob = 0x01000000
	createNewProcessGroup  = 0x00000200

	appExitTimeout   = 60 * time.Second
	installerTimeout = 10 * time.Minute
)

// ApplyRequest é tudo o que o atualizador precisa, passado por argumento.
type ApplyRequest struct {
	PID       int    // processo do app que vai sair
	Installer string // instalador já verificado
	SHA256    string // hash esperado do instalador, reconferido antes do UAC
	Size      int64  // tamanho esperado do instalador
	Dir       string // diretório da instalação atual
	Exe       string // nome do executável em Dir, relançado no fim
	Result    string // onde gravar o Result
	From, To  string // versões, só para o Result
}

// Result é o que o app lê no boot seguinte (var/update/resultado.json).
type Result struct {
	From  string `json:"de"`
	To    string `json:"para"`
	OK    bool   `json:"ok"`
	Error string `json:"erro,omitempty"`
}

var errAppAlive = errors.New("o HyPHP não fechou a tempo; a atualização não foi aplicada")

// Launch copia o executável em curso para updaterPath e o inicia em modo
// ApplyFlag, fora do Job Object. Quem chama só encerra o app depois que isto
// devolver nil: se falhar, o app continua aberto e o erro vai para a tela.
//
// A cópia é necessária porque o instalador sobrescreve o exe de req.Dir.
func Launch(req ApplyRequest, updaterPath string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("update: localizar o executável: %w", err)
	}
	if err := copyExe(self, updaterPath); err != nil {
		return fmt.Errorf("update: preparar o atualizador: %w", err)
	}
	cmd := exec.Command(updaterPath, append([]string{ApplyFlag}, req.args()...)...)
	cmd.Dir = filepath.Dir(updaterPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createBreakawayFromJob | createNewProcessGroup}
	if err := cmd.Start(); err != nil {
		// ERROR_ACCESS_DENIED aqui é o app rodando dentro de um job externo
		// que proíbe breakaway (algumas IDEs lançam assim).
		return fmt.Errorf("update: iniciar o atualizador fora do job do app: %w", err)
	}
	return cmd.Process.Release()
}

func (r ApplyRequest) args() []string {
	return []string{
		"--pid", strconv.Itoa(r.PID),
		"--instalador", r.Installer,
		"--sha256", r.SHA256,
		"--tamanho", strconv.FormatInt(r.Size, 10),
		"--dir", r.Dir,
		"--exe", r.Exe,
		"--resultado", r.Result,
		"--de", r.From,
		"--para", r.To,
	}
}

func parseApplyArgs(args []string) (ApplyRequest, error) {
	var req ApplyRequest
	fs := flag.NewFlagSet(ApplyFlag, flag.ContinueOnError)
	fs.IntVar(&req.PID, "pid", 0, "")
	fs.StringVar(&req.Installer, "instalador", "", "")
	fs.StringVar(&req.SHA256, "sha256", "", "")
	fs.Int64Var(&req.Size, "tamanho", 0, "")
	fs.StringVar(&req.Dir, "dir", "", "")
	fs.StringVar(&req.Exe, "exe", "", "")
	fs.StringVar(&req.Result, "resultado", "", "")
	fs.StringVar(&req.From, "de", "", "")
	fs.StringVar(&req.To, "para", "", "")
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return ApplyRequest{}, err
	}
	// Sem hash e tamanho não há como reconferir o instalador antes do UAC.
	if req.Result == "" || req.Dir == "" || req.Exe == "" || req.Installer == "" || req.SHA256 == "" || req.Size <= 0 {
		return ApplyRequest{}, errors.New("argumentos do atualizador incompletos")
	}
	return req, nil
}

// RunApply é o modo ApplyFlag: espera o app sair, roda o instalador e relança
// o app. Devolve o exit code do processo.
func RunApply(args []string) int {
	req, err := parseApplyArgs(args)
	if err != nil {
		return 2
	}
	logf := openApplyLog(filepath.Join(filepath.Dir(req.Result), "aplicar.log"))

	res := Result{From: req.From, To: req.To}
	err = apply(req, logf)
	if err != nil {
		res.Error = err.Error()
	} else {
		res.OK = true
	}
	logf("resultado: ok=%v erro=%q", res.OK, res.Error)
	if werr := writeResult(req.Result, res); werr != nil {
		logf("gravar resultado: %v", werr)
	}
	// Com o app antigo ainda aberto não há o que relançar — e abrir outro
	// cairia no single-instance dele.
	if errors.Is(err, errAppAlive) {
		return 1
	}
	exe := filepath.Join(req.Dir, req.Exe)
	cmd := exec.Command(exe)
	cmd.Dir = req.Dir
	if serr := cmd.Start(); serr != nil {
		logf("relançar %s: %v", exe, serr)
		return 1
	}
	_ = cmd.Process.Release()
	logf("relançado %s", exe)
	if err != nil {
		return 1
	}
	return 0
}

func apply(req ApplyRequest, logf func(string, ...any)) error {
	logf("aguardando o app (pid %d) sair", req.PID)
	if err := waitExit(req.PID, appExitTimeout); err != nil {
		return err
	}
	// var/update é gravável pelo usuário e a espera acima dura até 60 s: o
	// arquivo é reconferido logo antes de ser executado como administrador.
	if err := verifyFile(req.Installer, &Artifact{Size: req.Size, SHA256: req.SHA256}); err != nil {
		return err
	}
	// /D= precisa ser o último argumento e ir sem aspas, mesmo com espaços:
	// é regra do NSIS, e por isso a linha é montada à mão.
	params := "/S /D=" + req.Dir
	logf("rodando %s %s", req.Installer, params)
	code, err := elevate.RunInstaller(req.Installer, params, installerTimeout)
	switch {
	case errors.Is(err, elevate.ErrElevationDenied):
		return errors.New("atualização cancelada: a permissão do Windows (UAC) foi recusada")
	case err != nil:
		return fmt.Errorf("instalador: %w", err)
	case code != 0:
		return fmt.Errorf("o instalador terminou com código %d", code)
	}
	return nil
}

// waitExit espera o processo pid terminar. Processo que já não existe conta
// como terminado: o app pode ter saído antes de o atualizador chegar aqui.
func waitExit(pid int, timeout time.Duration) error {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)
	ev, err := windows.WaitForSingleObject(h, uint32(timeout/time.Millisecond))
	if err != nil {
		return fmt.Errorf("esperar o app sair: %w", err)
	}
	if ev != windows.WAIT_OBJECT_0 {
		return errAppAlive
	}
	return nil
}

func writeResult(path string, r Result) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// openApplyLog abre um log próprio: o atualizador não tem o logger do app, e
// sem ele uma falha entre "app fechou" e "app voltou" seria invisível.
func openApplyLog(path string) func(string, ...any) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return func(string, ...any) {}
	}
	return func(format string, a ...any) {
		fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, a...))
	}
}

func copyExe(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
