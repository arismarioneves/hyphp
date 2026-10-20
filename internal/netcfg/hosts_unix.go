//go:build darwin || linux

package netcfg

// HostsPath é o hosts do sistema. Variável: os testes do helper do Linux, que
// grava o hosts quando não há systemd-resolved, apontam para uma pasta
// temporária.
var HostsPath = "/etc/hosts"

// defaultEOL vale para hosts vazio: fora do Windows as linhas terminam em LF.
const defaultEOL = "\n"
