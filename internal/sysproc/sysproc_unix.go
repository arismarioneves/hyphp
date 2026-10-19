//go:build darwin || linux

package sysproc

import "os/exec"

// Hide não faz nada fora do Windows: processo filho não ganha janela de console.
func Hide(cmd *exec.Cmd) {}

// ExeName devolve o nome como veio: executáveis fora do Windows não têm sufixo.
func ExeName(base string) string { return base }
