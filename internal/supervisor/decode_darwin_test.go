package supervisor

import "testing"

// No macOS os serviços já escrevem UTF-8: decodeLine não pode reinterpretar
// a linha como faz com a página de código ANSI no Windows.
func TestDecodeLine_PreservaUTF8NoMacOS(t *testing.T) {
	casos := []string{
		"ready for connections",
		"Servidor iniciado na porta 8080",
		"Erro de conexão com o banco de dados: conexão recusada",
		"🚀 Inicializando runtime PHP",
		"",
	}
	for _, tc := range casos {
		if got := decodeLine([]byte(tc)); got != tc {
			t.Errorf("decodeLine(%q) = %q, quero idêntico", tc, got)
		}
	}
}
