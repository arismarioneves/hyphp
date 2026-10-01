// Package sysproc concentra o que muda por SO ao criar processos filhos.
package sysproc

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// createNoWindow é CREATE_NO_WINDOW da API do Windows.
//
// SysProcAttr.HideWindow sozinho não basta: ele só passa SW_HIDE pelo
// STARTUPINFO, e o console de um processo console é criado pelo kernel antes
// disso — a janela pisca na tela. Só esta flag impede a criação do console.
// Como o HyPHP roda sem console próprio, todo utilitário de vida curta que ele
// chama (php -v, mysql, mkcert, netstat) piscaria uma janela a cada varredura.
const createNoWindow = 0x08000000

// Hide impede que cmd abra uma janela de console. Preserva o que já houver em
// SysProcAttr (o editor usa CmdLine montada à mão).
func Hide(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

// ExeName devolve o nome do executável no Windows: base com ".exe".
func ExeName(base string) string {
	if strings.EqualFold(filepath.Ext(base), ".exe") {
		return base
	}
	return base + ".exe"
}
