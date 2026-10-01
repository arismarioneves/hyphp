//go:build windows

// Command hyphp-helper é o único binário elevado do HyPHP (spec §11). Faz três coisas que
// exigem administrador — gravar o arquivo hosts, registrar/remover regra NRPT e instalar a CA
// do mkcert — e nada além disso. Todo o conteúdo já chega pronto do hyphp.exe não-elevado.
//
// Saída: uma linha de JSON em stdout ({"ok":true} | {"ok":false,"error":"..."}) e, com
// --result <arquivo>, o mesmo JSON gravado nesse arquivo (o stdout de um processo elevado
// iniciado com SW_HIDE não chega a ninguém).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
	"hyphp/internal/sysproc"
)

// Exit codes do contrato C10.
const (
	exitOK    = 0
	exitUsage = 2 // subcomando/flag inválidos, ou conteúdo de hosts recusado
	exitIO    = 3 // falha de I/O ou do comando externo
)

const usage = "uso: hyphp-helper [--result <json>] <hosts-write|nrpt-add|nrpt-remove|mkcert-install> [flags]"

// nrptComment marca as regras NRPT criadas por nós. nrpt-remove só apaga regras com esta marca —
// o Cloudflare WARP instala regras próprias que não podem ser tocadas.
const nrptComment = "hyphp"

// namespaceRe evita injeção no script do PowerShell: sufixo DNS começa com ponto e só tem
// letras, dígitos, hífen e ponto.
var namespaceRe = regexp.MustCompile(`^\.[A-Za-z0-9.-]+$`)

type result struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func main() {
	args := os.Args[1:]
	resultPath := ""
	if len(args) >= 2 && args[0] == "--result" {
		resultPath, args = args[1], args[2:]
		// Elevado, gravar em qualquer caminho pedido seria uma escrita arbitrária
		// como administrador; o hyphp.exe sempre usa var/run da mesma raiz.
		if !under(resultPath, paths.Run()) {
			report("", exitUsage, fmt.Errorf("--result fora de %s: %q", paths.Run(), resultPath))
			os.Exit(exitUsage)
		}
	}
	code, err := run(args)
	report(resultPath, code, err)
	os.Exit(code)
}

// under responde se p está dentro de dir (comparação sem distinguir maiúsculas
// no Windows, por filepath.Rel). O próprio dir não conta.
func under(p, dir string) bool {
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(dir, abs)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
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

func run(args []string) (int, error) {
	if len(args) == 0 {
		return exitUsage, errors.New(usage)
	}
	switch args[0] {
	case "hosts-write":
		return hostsWrite(args[1:])
	case "nrpt-add":
		return nrptAdd(args[1:])
	case "nrpt-remove":
		return nrptRemove(args[1:])
	case "mkcert-install":
		return mkcertInstall(args[1:])
	default:
		return exitUsage, fmt.Errorf("subcomando desconhecido %q; %s", args[0], usage)
	}
}

// newFlags devolve um FlagSet mudo: a mensagem de erro vira JSON, não lixo em stderr.
func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// hostsWrite substitui o arquivo hosts pelo conteúdo de --from, que o hyphp.exe já renderizou
// com netcfg.RenderHostsBlock. Valida antes de encostar no arquivo: conteúdo sem cara de hosts
// é recusado com exit 2 e o arquivo original fica intacto.
func hostsWrite(argv []string) (int, error) {
	fs := newFlags("hosts-write")
	from := fs.String("from", "", "arquivo com o conteúdo final do hosts")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("hosts-write: %w", err)
	}
	if *from == "" {
		return exitUsage, errors.New("hosts-write: --from é obrigatório")
	}
	raw, err := os.ReadFile(*from)
	if err != nil {
		return exitIO, fmt.Errorf("hosts-write: ler %s: %w", *from, err)
	}
	if err := netcfg.ValidateHostsContent(string(raw)); err != nil {
		return exitUsage, fmt.Errorf("hosts-write: conteúdo recusado: %w", err)
	}
	if err := writeHostsAtomic(raw); err != nil {
		return exitIO, err
	}
	return exitOK, nil
}

// writeHostsAtomic grava em hosts.hyphp-tmp no mesmo diretório e renomeia por cima do hosts.
// os.Rename é MoveFileEx com MOVEFILE_REPLACE_EXISTING: ou troca o arquivo inteiro, ou não faz
// nada — nunca deixa um hosts pela metade. O arquivo costuma estar aberto por antivírus ou por
// um editor; nesse caso o rename falha com "sharing violation" e a gente tenta de novo.
func writeHostsAtomic(content []byte) error {
	dir := filepath.Dir(netcfg.HostsPath)
	tmp := filepath.Join(dir, "hosts.hyphp-tmp")
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return fmt.Errorf("hosts-write: gravar %s: %w", tmp, err)
	}
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = os.Rename(tmp, netcfg.HostsPath); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	os.Remove(tmp)
	return fmt.Errorf("hosts-write: substituir %s após 5 tentativas (arquivo em uso por antivírus ou editor?): %w", netcfg.HostsPath, err)
}

// nrptAdd registra a regra que manda as consultas do namespace para o nosso resolvedor.
// Remove antes uma eventual regra nossa do mesmo namespace: a operação é idempotente.
func nrptAdd(argv []string) (int, error) {
	fs := newFlags("nrpt-add")
	namespace := fs.String("namespace", "", "sufixo DNS, ex.: .test")
	server := fs.String("server", "", "IP do resolvedor, ex.: 127.0.0.1")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("nrpt-add: %w", err)
	}
	if *namespace == "" || *server == "" {
		return exitUsage, errors.New("nrpt-add: --namespace e --server são obrigatórios")
	}
	if !namespaceRe.MatchString(*namespace) {
		return exitUsage, fmt.Errorf("nrpt-add: --namespace deve começar com ponto e só ter letras, dígitos, hífen e ponto; veio %q", *namespace)
	}
	if net.ParseIP(*server) == nil {
		return exitUsage, fmt.Errorf("nrpt-add: --server não é um IP: %q", *server)
	}
	if out, err := powershell(removeRuleScript(*namespace)); err != nil {
		return exitIO, fmt.Errorf("nrpt-add (limpeza da regra anterior): %w: %s", err, out)
	}
	script := fmt.Sprintf(
		`Add-DnsClientNrptRule -Namespace '%s' -NameServers '%s' -Comment '%s' -ErrorAction Stop`,
		*namespace, *server, nrptComment)
	if out, err := powershell(script); err != nil {
		return exitIO, fmt.Errorf("nrpt-add: %w: %s", err, out)
	}
	return exitOK, nil
}

// nrptRemove apaga as regras do namespace criadas por nós. Não existir regra é sucesso.
func nrptRemove(argv []string) (int, error) {
	fs := newFlags("nrpt-remove")
	namespace := fs.String("namespace", "", "sufixo DNS, ex.: .test")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("nrpt-remove: %w", err)
	}
	if !namespaceRe.MatchString(*namespace) {
		return exitUsage, fmt.Errorf("nrpt-remove: --namespace inválido: %q", *namespace)
	}
	if out, err := powershell(removeRuleScript(*namespace)); err != nil {
		return exitIO, fmt.Errorf("nrpt-remove: %w: %s", err, out)
	}
	return exitOK, nil
}

// removeRuleScript apaga só as regras com o nosso comentário — regras de terceiros
// (o WARP instala as dele) ficam intactas. Sem regras casando, o script não faz nada.
func removeRuleScript(namespace string) string {
	return fmt.Sprintf(
		`$r = Get-DnsClientNrptRule | Where-Object { $_.Namespace -eq '%s' -and $_.Comment -eq '%s' }; if ($r) { $r | Remove-DnsClientNrptRule -Force -ErrorAction Stop }`,
		namespace, nrptComment)
}

// mkcertInstall roda `<exe> -install`, que gera a CA local e a instala na store do usuário
// do processo elevado. --caroot força o mesmo CAROOT que o hyphp.exe verifica em CAInstalled().
func mkcertInstall(argv []string) (int, error) {
	fs := newFlags("mkcert-install")
	exe := fs.String("exe", "", "caminho do mkcert.exe")
	caroot := fs.String("caroot", "", "diretório CAROOT (opcional)")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("mkcert-install: %w", err)
	}
	if *exe == "" {
		return exitUsage, errors.New("mkcert-install: --exe é obrigatório")
	}
	// O helper roda como administrador: sem esta trava ele executaria qualquer
	// binário indicado. Só o mkcert instalado pelo app, na raiz do helper, passa.
	mkcertDir := filepath.Join(paths.Bin(), "mkcert")
	if !strings.EqualFold(filepath.Base(*exe), "mkcert.exe") || !under(*exe, mkcertDir) {
		return exitUsage, fmt.Errorf("mkcert-install: --exe precisa ser mkcert.exe sob %s: %q", mkcertDir, *exe)
	}
	if _, err := os.Stat(*exe); err != nil {
		return exitUsage, fmt.Errorf("mkcert-install: %s não encontrado: %w", *exe, err)
	}
	cmd := exec.Command(*exe, "-install")
	sysproc.Hide(cmd)
	if *caroot != "" {
		cmd.Env = append(os.Environ(), "CAROOT="+*caroot)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return exitIO, fmt.Errorf("mkcert -install: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return exitOK, nil
}

// powershell roda um comando e devolve stdout+stderr aparados — a mensagem do cmdlet é a
// única pista útil quando a regra NRPT não entra.
func powershell(script string) (string, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	sysproc.Hide(cmd)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
