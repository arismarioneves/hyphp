// Package stack transforma (state + runtimes + projetos) em specs de processo,
// sites e pools, e aplica o resultado ao supervisor com validação prévia.
package stack

// Warning é um aviso não bloqueante mostrado na UI (evento stack:warnings).
type Warning struct {
	// Code: "port-conflict" | "php-missing" | "htaccess-under-nginx" |
	// "wildcard-unavailable" | "wildcard-pending" | "hosts-pending" |
	// "ca-pending" | "ports-pending" | "tls-unavailable" | "web-missing" |
	// "db-init-failed" | "db-create-failed" | "proc-exe-missing" |
	// "docroot-sem-indice" | "docroot-unsupported" | "cacert-missing"
	//
	// "hosts-pending", "ca-pending", "wildcard-pending" e "ports-pending" (só
	// no Linux) são as pendências que pedem privilégio. O Reconcile só as
	// reporta; a escrita fica em Stack.ApplyHosts, Stack.InstallCA,
	// Stack.ApplyWildcardDNS e Stack.ApplyPorts, disparadas por botão — abrir
	// o app não pede privilégio.
	Code      string `json:"code"`
	Message   string `json:"message"`
	ProjectID string `json:"projectId"` // "" quando global
}
