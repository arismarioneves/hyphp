package netcfg

import (
	"reflect"
	"strings"
	"testing"
)

func TestRenderHostsBlock(t *testing.T) {
	const bom = "\uFEFF"
	tests := []struct {
		name     string
		existing string
		domains  []string
		want     string
	}{
		{
			name:     "arquivo sem bloco: insere ao final com linha em branco, CRLF",
			existing: bom + "# Copyright (c) 1993-2009 Microsoft Corp.\r\n127.0.0.1\tfoo.local   # laragon\r\n",
			domains:  []string{"b.test", "a.test"},
			want:     bom + "# Copyright (c) 1993-2009 Microsoft Corp.\r\n127.0.0.1\tfoo.local   # laragon\r\n\r\n# hyphp:start\r\n127.0.0.1 a.test\r\n127.0.0.1 b.test\r\n# hyphp:end\r\n",
		},
		{
			name:     "arquivo sem EOL final: acrescenta EOL antes da linha em branco",
			existing: "127.0.0.1 x.local",
			domains:  []string{"a.test"},
			want:     "127.0.0.1 x.local\r\n\r\n# hyphp:start\r\n127.0.0.1 a.test\r\n# hyphp:end\r\n",
		},
		{
			name:     "arquivo LF: mantém LF",
			existing: "127.0.0.1 x.local\n",
			domains:  []string{"a.test"},
			want:     "127.0.0.1 x.local\n\n# hyphp:start\n127.0.0.1 a.test\n# hyphp:end\n",
		},
		{
			name:     "arquivo vazio: só o bloco",
			existing: "",
			domains:  []string{"a.test"},
			want:     "# hyphp:start" + defaultEOL + "127.0.0.1 a.test" + defaultEOL + "# hyphp:end" + defaultEOL,
		},
		{
			name:     "bloco no meio: substitui e preserva antes/depois byte a byte",
			existing: "# topo   \r\n\r\n# hyphp:start\r\n127.0.0.1 old.test\r\n# hyphp:end\r\n# rodapé\t\r\n192.168.0.1   nas  # comentário\r\n",
			domains:  []string{"new.test"},
			want:     "# topo   \r\n\r\n# hyphp:start\r\n127.0.0.1 new.test\r\n# hyphp:end\r\n# rodapé\t\r\n192.168.0.1   nas  # comentário\r\n",
		},
		{
			name:     "bloco vazio: preenche",
			existing: "# x\r\n# hyphp:start\r\n# hyphp:end\r\n",
			domains:  []string{"a.test"},
			want:     "# x\r\n# hyphp:start\r\n127.0.0.1 a.test\r\n# hyphp:end\r\n",
		},
		{
			name:     "domains vazio remove o bloco e a linha em branco anterior",
			existing: "127.0.0.1 x.local\r\n\r\n# hyphp:start\r\n127.0.0.1 a.test\r\n# hyphp:end\r\n",
			domains:  nil,
			want:     "127.0.0.1 x.local\r\n",
		},
		{
			name:     "domains vazio com bloco no meio: remove só o bloco",
			existing: "# a\r\n# hyphp:start\r\n127.0.0.1 a.test\r\n# hyphp:end\r\n# b\r\n",
			domains:  nil,
			want:     "# a\r\n# b\r\n",
		},
		{
			name:     "domains vazio sem bloco: inalterado",
			existing: "127.0.0.1 x.local\r\n",
			domains:  []string{},
			want:     "127.0.0.1 x.local\r\n",
		},
		{
			name:     "dedup, trim, minúsculas, ordenação",
			existing: "",
			domains:  []string{" B.test ", "a.test", "b.TEST", ""},
			want:     "# hyphp:start" + defaultEOL + "127.0.0.1 a.test" + defaultEOL + "127.0.0.1 b.test" + defaultEOL + "# hyphp:end" + defaultEOL,
		},
		{
			name:     "marcador no meio de uma linha não conta como bloco",
			existing: "# veja # hyphp:start no manual\r\n",
			domains:  []string{"a.test"},
			want:     "# veja # hyphp:start no manual\r\n\r\n# hyphp:start\r\n127.0.0.1 a.test\r\n# hyphp:end\r\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderHostsBlock(tt.existing, tt.domains)
			if got != tt.want {
				t.Fatalf("\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestParseHostsBlock(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		want     []string
	}{
		{"sem bloco", "127.0.0.1 x.local\r\n", nil},
		{"bloco com dois domínios", "# hyphp:start\r\n127.0.0.1 b.test\r\n127.0.0.1 a.test\r\n# hyphp:end\r\n", []string{"a.test", "b.test"}},
		{"ignora linhas fora do bloco e não-127", "127.0.0.1 fora.local\r\n# hyphp:start\r\n::1 seis.test\r\n127.0.0.1 a.test\r\n# hyphp:end\r\n127.0.0.1 depois.local\r\n", []string{"a.test"}},
		{"bloco vazio", "# hyphp:start\r\n# hyphp:end\r\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseHostsBlock(tt.existing)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderThenParseRoundTrip(t *testing.T) {
	domains := []string{"acme.test", "blog.test"}
	out := RenderHostsBlock("# x\r\n", domains)
	if got := ParseHostsBlock(out); !reflect.DeepEqual(got, domains) {
		t.Fatalf("round trip: got %v, want %v", got, domains)
	}
}

// Domínio com espaço, quebra de linha ou "#" viraria outra entrada no hosts
// gravado pelo helper elevado.
func TestRenderHostsBlockDescartaDominioInvalido(t *testing.T) {
	out := RenderHostsBlock("", []string{"ok.test", "evil.test\n203.0.113.5 www.banco.com", "minha loja.test", "a#b.test"})
	if got := ParseHostsBlock(out); !reflect.DeepEqual(got, []string{"ok.test"}) {
		t.Fatalf("domínios no bloco = %v, quero só ok.test:\n%s", got, out)
	}
	if strings.Contains(out, "203.0.113.5") {
		t.Fatalf("linha injetada no hosts:\n%s", out)
	}
}

func TestValidateHostsContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{"hosts padrão do Windows (só comentários, BOM)", "\uFEFF# Copyright\r\n#\t127.0.0.1       localhost\r\n", false},
		{"linhas ip host com comentário", "127.0.0.1      a.test        #laragon magic!   \r\n\r\n# hyphp:start\r\n127.0.0.1 b.test\r\n# hyphp:end\r\n", false},
		{"sem bloco (bloco removido) continua válido", "127.0.0.1 localhost\r\n", false},
		{"vazio", "", true},
		{"lixo: linha com um só campo", "banana\r\n", true},
		// O helper elevado é a única coisa que escreve no hosts: um arquivo de texto
		// qualquer com dois tokens não pode passar pela guarda.
		{"lixo: dois campos, primeiro não é IP", "lixo qualquer sem bloco\r\n", true},
		{"lixo: texto em prosa", "Este arquivo nao e um hosts\r\n", true},
		{"IPv6 é aceito", "::1 app.test\r\n", false},
		{"host com vários aliases", "127.0.0.1 a.test b.test c.test\r\n", false},
		{"comentário no fim da linha não invalida", "127.0.0.1 a.test # nota\r\n", false},
		{"NUL", "127.0.0.1 a\x00b\r\n", true},
		{"não UTF-8", "127.0.0.1 a\xff\r\n", true},
		// "Configuração" em Windows-1252: ç = 0xE7, ã = 0xE3.
		{"comentário em ANSI", "# Configura\xe7\xe3o da VPN\r\n127.0.0.1 localhost # m\xe1quina\r\n", false},
		{"não UTF-8 antes do comentário", "127.0.0.1 a\xff # ok\r\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostsContent(tt.content)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
