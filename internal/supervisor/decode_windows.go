//go:build windows

package supervisor

import (
	"unicode/utf8"

	"golang.org/x/sys/windows"
)

// cpACP é CP_ACP da API do Windows: "a code page ANSI corrente do sistema".
// x/sys/windows não exporta a constante.
const cpACP = 0

// decodeLine converte uma linha de saída de processo para UTF-8.
//
// Programas de console no Windows escrevem na code page do sistema (CP-1252 em
// português), não em UTF-8: o mysqld emite "Não foi possível encontrar o
// módulo" e os bytes chegam como 0xE3/0xED soltos. Tratados como UTF-8 viram
// U+FFFD e a tela de Logs mostra "N�o foi poss�vel" — ilegível justamente na
// mensagem de erro, que é quando o log importa.
//
// UTF-8 válido passa intacto: quem já emite UTF-8 (Mailpit, nginx) não é
// reinterpretado, e ASCII puro — a maioria esmagadora das linhas — sai pelo
// caminho rápido sem conversão nenhuma.
func decodeLine(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if utf8.Valid(b) {
		return string(b)
	}
	utf16, err := windows.MultiByteToWideChar(cpACP, 0, &b[0], int32(len(b)), nil, 0)
	if err != nil || utf16 <= 0 {
		return string(b)
	}
	buf := make([]uint16, utf16)
	if _, err := windows.MultiByteToWideChar(cpACP, 0, &b[0], int32(len(b)), &buf[0], utf16); err != nil {
		return string(b)
	}
	return windows.UTF16ToString(buf)
}
