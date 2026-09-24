// Package stack transforma (state + runtimes + projetos) em specs de processo,
// sites e pools, e aplica o resultado ao supervisor com validação prévia.
package stack

// Warning é um aviso não bloqueante mostrado na UI (evento stack:warnings).
type Warning struct {
	// Code: "port-conflict" | "php-missing" | "htaccess-under-nginx" |
	// "wildcard-unavailable" | "elevation-denied" | "tls-unavailable" | "web-missing"
	Code      string `json:"code"`
	Message   string `json:"message"`
	ProjectID string `json:"projectId"` // "" quando global
}
