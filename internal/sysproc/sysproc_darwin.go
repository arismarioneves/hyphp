package sysproc

import "os/exec"

// Hide não faz nada no macOS: processo filho não ganha janela de console.
func Hide(cmd *exec.Cmd) {}

// ExeName devolve o nome como veio: executáveis do macOS não têm sufixo.
func ExeName(base string) string { return base }
