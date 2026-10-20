//go:build darwin || linux

package elevate

import (
	"encoding/json"
	"strings"
	"time"
)

// helperTimeout é o prazo do Windows (helperTimeoutMS): uma janela de senha
// esquecida aberta não prende a ação para sempre.
const helperTimeout = 120 * time.Second

// helperFailure monta o erro do helper que saiu com code ≠ 0: a mensagem é o
// "error" da linha JSON que ele escreveu, ou a saída inteira, sem JSON.
func helperFailure(code int, out string) error {
	body := strings.TrimSpace(out)
	var res struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(body), &res) == nil && res.Error != "" {
		return &HelperError{ExitCode: code, Message: res.Error}
	}
	return &HelperError{ExitCode: code, Message: body}
}
