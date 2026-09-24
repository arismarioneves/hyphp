package netcfg

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const HostsPath = `C:\Windows\System32\drivers\etc\hosts`
const blockStart = "# hyphp:start"
const blockEnd = "# hyphp:end"

// maxHostsSize limita o tamanho aceito por ValidateHostsContent (1 MiB).
const maxHostsSize = 1 << 20

var hostsLineRe = regexp.MustCompile(`^\s*\S+\s+\S+`)

// detectEOL devolve o terminador de linha do arquivo: o primeiro encontrado.
// Sem nenhum terminador (arquivo vazio ou de uma linha) assume CRLF, o padrão do Windows.
func detectEOL(s string) string {
	i := strings.IndexByte(s, '\n')
	if i > 0 && s[i-1] == '\r' {
		return "\r\n"
	}
	if i >= 0 {
		return "\n"
	}
	return "\r\n"
}

// normalizeDomains devolve os domínios em minúsculas, sem vazios, sem duplicatas, ordenados.
func normalizeDomains(domains []string) []string {
	seen := make(map[string]struct{}, len(domains))
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if _, dup := seen[d]; dup {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// findBlock localiza o bloco [start, end) em s, onde start é o início da linha
// "# hyphp:start" e end é o primeiro byte após a linha "# hyphp:end" (incluindo seu EOL).
// Se o marcador final estiver ausente, o bloco vai até o fim de s.
func findBlock(s string) (start, end int, ok bool) {
	start = indexLine(s, blockStart, 0)
	if start < 0 {
		return 0, 0, false
	}
	endLine := indexLine(s, blockEnd, start+len(blockStart))
	if endLine < 0 {
		return start, len(s), true
	}
	end = endLine + len(blockEnd)
	// consome o EOL da linha "# hyphp:end"
	if strings.HasPrefix(s[end:], "\r\n") {
		end += 2
	} else if strings.HasPrefix(s[end:], "\n") {
		end++
	}
	return start, end, true
}

// indexLine devolve o índice do primeiro marker que começa uma linha (índice 0 ou após '\n')
// e é seguido de EOL ou fim de string. Ignora ocorrências no meio de uma linha.
func indexLine(s, marker string, from int) int {
	for {
		i := strings.Index(s[from:], marker)
		if i < 0 {
			return -1
		}
		i += from
		atLineStart := i == 0 || s[i-1] == '\n'
		rest := s[i+len(marker):]
		atLineEnd := rest == "" || strings.HasPrefix(rest, "\r\n") || strings.HasPrefix(rest, "\n")
		if atLineStart && atLineEnd {
			return i
		}
		from = i + len(marker)
	}
}

// RenderHostsBlock devolve o conteúdo do hosts com o bloco hyphp substituído ou inserido.
// Tudo fora do bloco é preservado byte a byte (BOM, comentários, espaços, terminador de linha).
// domains vazio remove o bloco inteiro (e a linha em branco que o precede).
// Só gera linhas IPv4 (127.0.0.1) — sem ::1, para o navegador não tentar IPv6 primeiro.
func RenderHostsBlock(existing string, domains []string) string {
	eol := detectEOL(existing)
	domains = normalizeDomains(domains)

	var block string
	if len(domains) > 0 {
		var b strings.Builder
		b.WriteString(blockStart)
		b.WriteString(eol)
		for _, d := range domains {
			b.WriteString("127.0.0.1 ")
			b.WriteString(d)
			b.WriteString(eol)
		}
		b.WriteString(blockEnd)
		b.WriteString(eol)
		block = b.String()
	}

	start, end, found := findBlock(existing)
	if !found {
		if block == "" {
			return existing
		}
		if existing == "" {
			return block
		}
		var b strings.Builder
		b.WriteString(existing)
		if !strings.HasSuffix(existing, "\n") {
			b.WriteString(eol)
		}
		b.WriteString(eol) // linha em branco separando o bloco do conteúdo alheio
		b.WriteString(block)
		return b.String()
	}

	before := existing[:start]
	after := existing[end:]
	if block == "" && strings.HasSuffix(before, eol+eol) {
		// remove a linha em branco que nós mesmos inserimos antes do bloco
		before = before[:len(before)-len(eol)]
	}
	return before + block + after
}

// ParseHostsBlock devolve os domínios listados no bloco hyphp (ordenados, sem duplicatas).
// Ausência de bloco ou bloco vazio → nil.
func ParseHostsBlock(existing string) []string {
	start, end, found := findBlock(existing)
	if !found {
		return nil
	}
	var domains []string
	for _, line := range strings.Split(existing[start:end], "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "127.0.0.1" {
			continue
		}
		domains = append(domains, fields[1])
	}
	if len(domains) == 0 {
		return nil
	}
	return normalizeDomains(domains)
}

// ValidateHostsContent aceita apenas conteúdo com cara de arquivo hosts: não vazio, ≤ 1 MiB,
// UTF-8 válido, sem NUL, e toda linha não-vazia/não-comentário no formato "<ip> <host> [...]".
// É a defesa do hyphp-helper contra sobrescrever o hosts com lixo.
func ValidateHostsContent(content string) error {
	if content == "" {
		return errors.New("conteúdo vazio")
	}
	if len(content) > maxHostsSize {
		return fmt.Errorf("conteúdo excede %d bytes", maxHostsSize)
	}
	if !utf8.ValidString(content) {
		return errors.New("conteúdo não é UTF-8 válido")
	}
	if strings.IndexByte(content, 0) >= 0 {
		return errors.New("conteúdo contém byte NUL")
	}
	content = strings.TrimPrefix(content, "\uFEFF")
	for n, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !hostsLineRe.MatchString(line) {
			return fmt.Errorf("linha %d não tem formato \"<ip> <host>\": %q", n+1, line)
		}
	}
	return nil
}
