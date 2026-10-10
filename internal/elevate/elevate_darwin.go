package elevate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hyphp/internal/i18n"
)

// helperTimeout é o prazo do Windows (helperTimeoutMS): uma janela de senha
// esquecida aberta não prende a ação para sempre.
const helperTimeout = 120 * time.Second

// adminScript recebe em argv o texto da janela, o helper e os argumentos, e
// cita cada um com `quoted form of`: nada de fora vira código de shell. O
// printf final devolve o código de saída do helper no stdout, porque o `do
// shell script` descarta o stdout quando o comando falha; o 2>&1 traz junto
// qualquer erro do helper.
var adminScript = []string{
	"on run argv",
	`set cmd to ""`,
	"repeat with i from 2 to count of argv",
	`set cmd to cmd & quoted form of (item i of argv) & " "`,
	"end repeat",
	`return do shell script (cmd & "2>&1; printf '\\nexit=%d' $?") with prompt (item 1 of argv) with administrator privileges without altering line endings`,
	"end run",
}

// osascriptArgs monta a linha do osascript: o script em -e e, depois do "--",
// prompt, exe e args como argv.
func osascriptArgs(prompt, exe string, args []string) []string {
	out := make([]string, 0, 2*len(adminScript)+3+len(args))
	for _, line := range adminScript {
		out = append(out, "-e", line)
	}
	out = append(out, "--", prompt, exe)
	return append(out, args...)
}

// RunElevated roda exe com args como root, pela janela de senha do macOS.
//
// Erros: ErrElevationDenied (janela cancelada), ErrHelperTimeout (passou de
// 2 min), *HelperError (o helper saiu com código ≠ 0, ou o osascript falhou).
func RunElevated(exe string, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", osascriptArgs(i18n.T("elevate.prompt"), exe, args)...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	switch {
	case ctx.Err() != nil:
		return ErrHelperTimeout
	case err != nil:
		return osascriptError(stderr.String())
	}
	return helperResult(stdout.String())
}

// osascriptError trata a falha do próprio osascript: -128 é a janela de senha
// cancelada.
func osascriptError(stderr string) error {
	if strings.Contains(stderr, "(-128)") {
		return ErrElevationDenied
	}
	return &HelperError{ExitCode: -1, Message: strings.TrimSpace(stderr)}
}

// helperResult lê o que o do shell script devolveu: a linha JSON do helper e,
// por último, "exit=<código>".
func helperResult(out string) error {
	out = strings.TrimRight(out, "\r\n")
	i := strings.LastIndex(out, "exit=")
	if i < 0 {
		return &HelperError{ExitCode: -1, Message: "saída do helper sem código: " + out}
	}
	code, err := strconv.Atoi(strings.TrimSpace(out[i+len("exit="):]))
	if err != nil {
		return &HelperError{ExitCode: -1, Message: "saída do helper sem código: " + out}
	}
	if code == 0 {
		return nil
	}
	body := strings.TrimSpace(out[:i])
	var res struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(body), &res) == nil && res.Error != "" {
		return &HelperError{ExitCode: code, Message: res.Error}
	}
	return &HelperError{ExitCode: code, Message: body}
}

// HelperPath devolve o hyphp-helper do bundle em execução.
func HelperPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("HelperPath: %w", err)
	}
	return helperPathFor(exe)
}

// helperPathFor: o helper fica em Contents/Helpers, ao lado da CLI (mesma
// conta do cliExe de platform_darwin.go, na raiz).
func helperPathFor(appExe string) (string, error) {
	p := filepath.Clean(filepath.Join(filepath.Dir(appExe), "..", "Helpers", "hyphp-helper"))
	st, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("HelperPath: %s ausente (rode `wails3 task darwin:build:helper`): %w", p, err)
	}
	if st.IsDir() {
		return "", fmt.Errorf("HelperPath: %s é um diretório", p)
	}
	return p, nil
}

// RunInstaller ainda não existe no macOS: a aplicação de update é da M3.
func RunInstaller(exe, params string, timeout time.Duration) (uint32, error) {
	return 0, i18n.Errorf("err.mac.unavailable")
}
