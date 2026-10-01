package stack

import (
	"slices"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// nrptKey guarda as regras NRPT locais, uma subchave {GUID} por regra, com o
// namespace em Name (REG_MULTI_SZ) e o comentário em Comment. As de GPO moram
// em outra chave e nunca são nossas.
const nrptKey = `SYSTEM\CurrentControlSet\Services\Dnscache\Parameters\DnsPolicyConfig`

// nrptComment é a marca que o cmd/hyphp-helper põe nas regras que cria (o
// mesmo valor de lá); regras de terceiros para o mesmo namespace, como as do
// WARP, não contam como a nossa.
const nrptComment = "hyphp"

// nrptRuleExists responde se a regra do HyPHP para namespace já está no
// sistema. A regra sobrevive ao app, e sem esta consulta o aviso
// wildcard-pending (com o botão de UAC) voltava em toda abertura. Lê o
// registro, e não Get-DnsClientNrptRule, porque a leitura não exige admin e
// o Reconcile não deveria pagar um PowerShell a cada execução.
func nrptRuleExists(namespace string) bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, nrptKey, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return false
	}
	defer k.Close()
	subs, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(subs, func(sub string) bool {
		return nrptRuleMatches(k, sub, namespace)
	})
}

func nrptRuleMatches(parent registry.Key, sub, namespace string) bool {
	k, err := registry.OpenKey(parent, sub, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	if comment, _, err := k.GetStringValue("Comment"); err != nil || comment != nrptComment {
		return false
	}
	names, _, err := k.GetStringsValue("Name")
	if err != nil {
		return false
	}
	return slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, namespace) })
}
