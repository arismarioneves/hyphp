package netcfg

// HostsPath é o hosts do Windows, escrito pelo helper elevado.
const HostsPath = `C:\Windows\System32\drivers\etc\hosts`

// defaultEOL vale para hosts vazio ou de uma linha: CRLF, o padrão do Windows.
const defaultEOL = "\r\n"
