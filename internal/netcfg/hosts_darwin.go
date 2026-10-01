package netcfg

// HostsPath é o hosts do macOS. Na M0 só é lido (para o aviso de hosts
// pendente); a escrita com senha de admin chega na M2.
const HostsPath = "/etc/hosts"

// defaultEOL vale para hosts vazio: no macOS as linhas terminam em LF.
const defaultEOL = "\n"
