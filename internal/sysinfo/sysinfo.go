// Package sysinfo descreve o sistema em que o HyPHP roda, para a tela Sobre e
// o diagnóstico que o usuário cola numa issue.
package sysinfo

import "runtime"

// Arch é a arquitetura do binário com o nome que o usuário conhece:
// "x64" no lugar de "amd64".
func Arch() string {
	if runtime.GOARCH == "amd64" {
		return "x64"
	}
	return runtime.GOARCH
}
