// Command hyphp-helper é o único binário do HyPHP que roda com privilégio de
// administrador (spec §11). No Windows sobe pelo UAC e grava hosts, regra NRPT
// e a CA do mkcert; no Mac sobe pelo osascript e grava a regra de DNS, o
// /etc/paths.d e a confiança na CA. Nada além disso: o conteúdo é montado ou
// validado aqui, a partir de flags simples.
//
// Saída: uma linha de JSON em stdout ({"ok":true} | {"ok":false,"error":"..."}).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"os"
)

// Exit codes do contrato C10.
const (
	exitOK    = 0
	exitUsage = 2 // subcomando/flag inválidos, ou conteúdo recusado
	exitIO    = 3 // falha de I/O ou do comando externo
)

type result struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// report emite o JSON em stdout e, se pedido, no arquivo de resultado.
func report(resultPath string, code int, err error) {
	res := result{OK: code == exitOK}
	if err != nil {
		res.Error = err.Error()
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf) // Encode já acrescenta o \n final
	// Sem isto, "<json>" da mensagem de uso vira "\u003cjson\u003e" e o erro fica ilegível.
	enc.SetEscapeHTML(false)
	if encErr := enc.Encode(res); encErr != nil { // não deve acontecer: a struct é trivial
		buf.Reset()
		buf.WriteString(`{"ok":false,"error":"falha ao serializar o resultado"}` + "\n")
	}
	os.Stdout.Write(buf.Bytes())
	if resultPath != "" {
		_ = os.WriteFile(resultPath, buf.Bytes(), 0o644)
	}
}

// newFlags devolve um FlagSet mudo: a mensagem de erro vira JSON, não lixo em stderr.
func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}
