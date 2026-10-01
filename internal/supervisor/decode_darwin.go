package supervisor

// decodeLine no macOS devolve a linha como veio: os serviços escrevem UTF-8,
// sem a página de código ANSI que o Windows usa no console.
func decodeLine(b []byte) string { return string(b) }
