package stack

// SystemChanges é o que o HyPHP gravou fora da própria pasta, para o "Remover
// do sistema" (no Mac e no Linux; no Windows Supported é falso e o resto não
// se aplica).
type SystemChanges struct {
	Supported bool `json:"supported"`
	DNS       bool `json:"dns"`   // regra de DNS do .test com a marca do HyPHP (/etc/resolver/test, drop-in do resolved)
	Path      bool `json:"path"`  // /etc/paths.d/hyphp ou /etc/profile.d/hyphp.sh com a pasta cli
	CA        bool `json:"ca"`    // CA do mkcert confiável para o sistema
	Ports     bool `json:"ports"` // limite das portas sem root baixado pelo HyPHP (Linux)
	Hosts     bool `json:"hosts"` // domínios no bloco do HyPHP no /etc/hosts (Linux sem systemd-resolved)
}
