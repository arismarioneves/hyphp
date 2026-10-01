package update

import "strconv"

// ApplyFlag é o primeiro argumento do modo que aplica o update. main.go o
// trata antes de tudo: nesse modo não há log em paths.Log(), nem Wails, nem
// paths.Root() — a cópia roda de var/update/, e a raiz calculada a partir
// dela seria a pasta errada.
const ApplyFlag = "--aplicar-update"

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
