// Package state persiste as preferências globais do HyPHP em var/state.json.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WebServerName identifica o web server ativo.
type WebServerName string

const (
	Apache WebServerName = "apache"
	Nginx  WebServerName = "nginx"
)

// State é o conteúdo de state.json. Campos ausentes no arquivo mantêm o valor de Default().
type State struct {
	SchemaVersion   int           `json:"schemaVersion"`   // 1
	WebServer       WebServerName `json:"webServer"`       // default "apache"
	DefaultPHP      string        `json:"defaultPhp"`      // "8.1"; vazio = maior instalada
	PoolSize        int           `json:"poolSize"`        // default 4
	HTTPPort        int           `json:"httpPort"`        // 80
	HTTPSPort       int           `json:"httpsPort"`       // 443
	MySQLPort       int           `json:"mysqlPort"`       // 3306
	MailpitSMTPPort int           `json:"mailpitSmtpPort"` // 1025
	MailpitHTTPPort int           `json:"mailpitHttpPort"` // 8025
	PhpMyAdminPort  int           `json:"phpMyAdminPort"`  // 8036
	// PhpMyAdminSecret é gerado uma vez e persistido: regerar a cada Reconcile
	// invalidaria a sessão aberta do usuário a cada mudança de projeto.
	PhpMyAdminSecret string              `json:"phpMyAdminSecret"`
	Roots            []string            `json:"roots"`                   // diretórios-raiz de projetos
	PortAlloc        map[string][]int    `json:"portAlloc"`               // "php:8.1" → portas reservadas
	PHPExtensions    map[string][]string `json:"phpExtensions,omitempty"` // série "8.1" → extensões habilitadas; ausente = runtime.DefaultExtensions
	Editor           string              `json:"editor"`                  // caminho do exe ou "" (usa `code`)
	Terminal         string              `json:"terminal"`                // "" = wt.exe se existir, senão cmd
	SidebarCollapsed bool                `json:"sidebarCollapsed"`
	Autostart        bool                `json:"autostart"`
	// AutoUpdateOff desliga a verificação periódica de versões. Invertido de
	// propósito: o zero-value (ausente em state.json antigo) significa ligado.
	AutoUpdateOff bool `json:"autoUpdateOff"`
}

// Default é o estado da primeira execução.
func Default() State {
	return State{
		SchemaVersion:   1,
		WebServer:       Apache,
		DefaultPHP:      "",
		PoolSize:        4,
		HTTPPort:        80,
		HTTPSPort:       443,
		MySQLPort:       3306,
		MailpitSMTPPort: 1025,
		MailpitHTTPPort: 8025,
		PhpMyAdminPort:  8036,
		Roots:           []string{},
		PortAlloc:       map[string][]int{},
	}
}

// Load lê o arquivo. Ausente → Default(), nil. Campos ausentes no JSON ficam com o Default.
func Load(path string) (State, error) {
	s := Default()
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("state: ler %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return State{}, fmt.Errorf("state: decodificar %s: %w", path, err)
	}
	if s.Roots == nil {
		s.Roots = []string{}
	}
	if s.PortAlloc == nil {
		s.PortAlloc = map[string][]int{}
	}
	return s, nil
}

// Save escreve atomicamente: grava path+".tmp" e renomeia por cima do destino.
// Cria o diretório pai se não existir.
func Save(path string, s State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("state: criar diretorio de %s: %w", path, err)
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("state: codificar: %w", err)
	}
	raw = append(raw, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("state: escrever %s: %w", tmp, err)
	}
	// os.Rename no Windows usa MOVEFILE_REPLACE_EXISTING: substitui o destino.
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("state: renomear %s → %s: %w", tmp, path, err)
	}
	return nil
}
