//go:build darwin || linux

package netcfg

// HostsPath é o hosts do sistema.
const HostsPath = "/etc/hosts"

// defaultEOL vale para hosts vazio: fora do Windows as linhas terminam em LF.
const defaultEOL = "\n"
