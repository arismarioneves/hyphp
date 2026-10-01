// Package state persiste as preferências globais do HyPHP em var/state.json.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WebServerName identifica o web server ativo.
type WebServerName string

const (
	Apache WebServerName = "apache"
	Nginx  WebServerName = "nginx"
)

// Valores de State.Theme.
const (
	ThemeDark   = "dark"
	ThemeLight  = "light"
	ThemeSystem = "system"
)

// Valores de State.DBEngine.
const (
	DBMySQL   = "mysql"
	DBMariaDB = "mariadb"
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
	// PHPIni são as diretivas que o usuário definiu por série ("8.3" →
	// "max_input_vars" → "5000"). Vão para o fim do php.ini, depois dos
	// padrões do HyPHP, e por isso passam por cima deles.
	PHPIni           map[string]map[string]string `json:"phpIni,omitempty"`
	Editor           string                       `json:"editor"`   // caminho do exe ou "" (usa `code`)
	Terminal         string                       `json:"terminal"` // "" = wt.exe se existir, senão cmd
	SidebarCollapsed bool                         `json:"sidebarCollapsed"`
	Autostart        bool                         `json:"autostart"`
	// AutoUpdateOff desliga a verificação periódica de versões. Invertido de
	// propósito: o zero-value (ausente em state.json antigo) significa ligado.
	AutoUpdateOff bool `json:"autoUpdateOff"`
	// Theme é o tema da interface: "dark", "light" ou "system" (segue o
	// Windows). O zero-value, de state.json anterior à v3, vale como "dark",
	// que era o único tema.
	Theme string `json:"theme"`
	// Language é o idioma da interface ("pt-BR", "en"). Vazio segue o idioma
	// do Windows (i18n.Resolve).
	Language string `json:"language"`
	// DBEngine é o banco ativo: "mysql" ou "mariadb". Um de cada vez, na
	// MySQLPort, cada um com o próprio datadir. Vazio vale "mysql", o único
	// até a v3.
	DBEngine string `json:"dbEngine"`
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
		return State{}, &decodeError{path: path, err: err}
	}
	if s.Roots == nil {
		s.Roots = []string{}
	}
	if s.PortAlloc == nil {
		s.PortAlloc = map[string][]int{}
	}
	return s, nil
}

// decodeError separa "o arquivo existe mas não é um state válido" de erro de
// E/S: só o primeiro é recuperável descartando o arquivo.
type decodeError struct {
	path string
	err  error
}

func (e *decodeError) Error() string {
	return fmt.Sprintf("state: decodificar %s: %v", e.path, e.err)
}

func (e *decodeError) Unwrap() error { return e.err }

// LoadOrRecover é o Load do boot. Um state.json que não decodifica (truncado
// ou zerado por queda de energia) impedia o app de abrir até o usuário achar
// e apagar o arquivo; como tudo nele é preferência regenerável, o arquivo vai
// para <path>.corrupt-<AAAAMMDD-HHMMSS> — guardado para diagnóstico — e o boot
// segue com Default(). recovered é o caminho do arquivo separado, vazio se
// nada foi recuperado. Erro de E/S continua sendo erro: descartar um arquivo
// que só não pôde ser lido apagaria preferências válidas.
func LoadOrRecover(path string) (st State, recovered string, err error) {
	st, err = Load(path)
	var de *decodeError
	if !errors.As(err, &de) {
		return st, "", err
	}
	recovered = path + ".corrupt-" + time.Now().Format("20060102-150405")
	if rerr := os.Rename(path, recovered); rerr != nil {
		return State{}, "", fmt.Errorf("state: separar %s corrompido: %w", path, rerr)
	}
	return Default(), recovered, nil
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
	if err := writeSynced(tmp, raw); err != nil {
		return fmt.Errorf("state: escrever %s: %w", tmp, err)
	}
	// os.Rename no Windows usa MOVEFILE_REPLACE_EXISTING: substitui o destino.
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("state: renomear %s → %s: %w", tmp, path, err)
	}
	return nil
}

// writeSynced grava e força o conteúdo para o disco antes de o Save renomear.
// Sem o Sync, o rename pode chegar ao disco antes dos dados, e uma queda de
// energia logo depois deixava state.json com 0 bytes ou truncado.
func writeSynced(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
