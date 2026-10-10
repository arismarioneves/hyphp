package netcfg

import "testing"

// A regra do HyPHP se reconhece pela marca na primeira linha; arquivo de outro
// programa (Laravel Valet, dnsmasq do usuário) nunca conta como nosso, e a
// porta ausente é a 53 do resolver(5).
func TestRegraDoResolvedor(t *testing.T) {
	for _, c := range []struct {
		nome     string
		conteudo string
		nossa    bool
		porta    int
	}{
		{"gravada pelo HyPHP", RenderResolverRule(15353), true, 15353},
		{"nossa com outra porta", RenderResolverRule(5300), true, 5300},
		{"do Valet", "nameserver 127.0.0.1\n", false, 53},
		{"marca fora da primeira linha", "nameserver 127.0.0.1\n# hyphp\nport 15353\n", false, 15353},
		{"vazio", "", false, 53},
	} {
		t.Run(c.nome, func(t *testing.T) {
			nossa, porta := ParseResolverRule(c.conteudo)
			if nossa != c.nossa || porta != c.porta {
				t.Fatalf("ParseResolverRule = (%v, %d), quer (%v, %d)", nossa, porta, c.nossa, c.porta)
			}
		})
	}
	if got, want := RenderResolverRule(15353), "# hyphp\nnameserver 127.0.0.1\nport 15353\n"; got != want {
		t.Fatalf("RenderResolverRule = %q, quer %q", got, want)
	}
}
