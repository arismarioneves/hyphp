package stack

// SystemChanges é o que o HyPHP gravou fora da própria pasta, para o "Remover
// do sistema" (só no Mac; no Windows Supported é falso e o resto não se aplica).
type SystemChanges struct {
	Supported bool `json:"supported"`
	DNS       bool `json:"dns"`  // /etc/resolver/test com a marca do HyPHP
	Path      bool `json:"path"` // /etc/paths.d/hyphp com a pasta cli
	CA        bool `json:"ca"`   // CA do mkcert confiável no keychain do sistema
}
