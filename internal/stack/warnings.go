// Package stack transforma (state + runtimes + projetos) em specs de processo,
// sites e pools, e aplica o resultado ao supervisor com validação prévia.
package stack

// Warning é um aviso não bloqueante mostrado na UI (evento stack:warnings).
type Warning struct {
	// Code: "port-conflict" | "php-missing" | "htaccess-under-nginx" |
	// "wildcard-unavailable" | "wildcard-pending" | "hosts-pending" |
	// "ca-pending" | "tls-unavailable" | "web-missing" | "db-init-failed" |
	// "db-create-failed" | "proc-exe-missing"
	//
	// "hosts-pending", "ca-pending" e "wildcard-pending" são as três pendências
	// que exigem UAC. O Reconcile só as reporta; a escrita fica em
	// Stack.ApplyHosts, Stack.InstallCA e Stack.ApplyWildcardDNS, disparadas
	// por botão — abrir o app não pede privilégio.
	Code      string `json:"code"`
	Message   string `json:"message"`
	ProjectID string `json:"projectId"` // "" quando global
}
