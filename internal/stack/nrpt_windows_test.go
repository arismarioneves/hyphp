package stack

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// Só a regra com a nossa marca para o namespace conta: uma regra do WARP para
// .test, ou a nossa para outro sufixo, deixaria o usuário sem resolução e sem
// o aviso que leva ao botão. As regras reais ficam em HKLM; o critério é o
// mesmo numa chave temporária do usuário.
func TestNRPTRuleMatches(t *testing.T) {
	base := fmt.Sprintf(`Software\hyphp-test-nrpt-%d`, time.Now().UnixNano())
	parent, _, err := registry.CreateKey(registry.CURRENT_USER, base, registry.ALL_ACCESS)
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	t.Cleanup(func() {
		subs, _ := parent.ReadSubKeyNames(-1)
		for _, s := range subs {
			_ = registry.DeleteKey(parent, s)
		}
		parent.Close()
		_ = registry.DeleteKey(registry.CURRENT_USER, base)
	})

	regra := func(sub, comment string, names ...string) {
		t.Helper()
		k, _, err := registry.CreateKey(parent, sub, registry.ALL_ACCESS)
		if err != nil {
			t.Fatalf("CreateKey %s: %v", sub, err)
		}
		defer k.Close()
		if err := k.SetStringValue("Comment", comment); err != nil {
			t.Fatal(err)
		}
		if err := k.SetStringsValue("Name", names); err != nil {
			t.Fatal(err)
		}
	}
	regra("nossa", "hyphp", ".TEST")
	regra("warp", "WARP", ".test")
	regra("outro-sufixo", "hyphp", ".local")

	casos := []struct {
		sub  string
		want bool
	}{
		{"nossa", true},
		{"warp", false},
		{"outro-sufixo", false},
		{"inexistente", false},
	}
	for _, c := range casos {
		if got := nrptRuleMatches(parent, c.sub, dnsSuffix); got != c.want {
			t.Errorf("nrptRuleMatches(%s) = %v, quero %v", c.sub, got, c.want)
		}
	}
}
