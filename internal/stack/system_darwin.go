package stack

// manageHosts: no Mac os domínios .test resolvem pela regra de DNS
// (/etc/resolver/test); o /etc/hosts não é gravado nem conferido.
func manageHosts() bool { return false }
