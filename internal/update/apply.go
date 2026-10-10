package update

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ApplyFlag é o primeiro argumento do modo que aplica o update. main.go o
// trata antes de tudo: nesse modo não há log em paths.Log(), nem Wails, nem
// paths.Root() — a cópia roda de var/update/, e a raiz calculada a partir
// dela seria a pasta errada.
const ApplyFlag = "--aplicar-update"

// ApplyRequest é tudo o que o atualizador precisa, passado por argumento.
type ApplyRequest struct {
	PID       int    // processo do app que vai sair
	Installer string // pacote já verificado: o instalador no Windows, o dmg no Mac
	SHA256    string // hash esperado do pacote, reconferido logo antes de usar
	Size      int64  // tamanho esperado do pacote
	Dir       string // pasta do executável atual (no Mac, Contents/MacOS do bundle)
	Exe       string // nome do executável em Dir
	Result    string // onde gravar o Result
	From, To  string // versões; no Mac, To é também a que o dmg tem de trazer
}

// Result é o que o app lê no boot seguinte (var/update/resultado.json).
type Result struct {
	From  string `json:"de"`
	To    string `json:"para"`
	OK    bool   `json:"ok"`
	Error string `json:"erro,omitempty"`
}

// appExitTimeout é quanto o atualizador espera o app fechar.
const appExitTimeout = 60 * time.Second

var errAppAlive = errors.New("o HyPHP não fechou a tempo; a atualização não foi aplicada")

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
	// Sem hash e tamanho não há como reconferir o pacote antes de usá-lo.
	if req.Result == "" || req.Dir == "" || req.Exe == "" || req.Installer == "" || req.SHA256 == "" || req.Size <= 0 {
		return ApplyRequest{}, errors.New("argumentos do atualizador incompletos")
	}
	return req, nil
}

// RunApply é o modo ApplyFlag: espera o app sair, aplica o pacote e relança
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
	alvo, serr := relaunch(req)
	if serr != nil {
		logf("relançar %s: %v", alvo, serr)
		return 1
	}
	logf("relançado %s", alvo)
	if err != nil {
		return 1
	}
	return 0
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
