// Package project descobre projetos em diretórios-raiz, lê e grava hyphp.yaml
// e observa mudanças. Não conhece supervisor nem web server.
package project

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// ManifestFile é o nome do manifesto na raiz de cada projeto.
const ManifestFile = "hyphp.yaml"

// Manifest é o conteúdo de hyphp.yaml (C15). As json tags existem para o
// binding TS de Project sair camelCase.
type Manifest struct {
	Name       string            `yaml:"name" json:"name"`
	Domain     string            `yaml:"domain,omitempty" json:"domain"`
	Wildcard   bool              `yaml:"wildcard,omitempty" json:"wildcard"`
	PHP        string            `yaml:"php,omitempty" json:"php"`
	Docroot    string            `yaml:"docroot,omitempty" json:"docroot"`
	Extensions []string          `yaml:"extensions,omitempty" json:"extensions"`
	Database   string            `yaml:"database,omitempty" json:"database"`
	Processes  map[string]string `yaml:"processes,omitempty" json:"processes"`
}

var (
	phpMajorRe = regexp.MustCompile(`^\d+\.\d+$`)
	// domainRe aceita rótulos de hostname (RFC 1123) separados por ponto.
	// Sem diferenciar caixa: DNS e hosts não diferenciam, e recusar
	// "Loja.test" quebraria manifestos que já funcionavam.
	domainRe    = regexp.MustCompile(`(?i)^([a-z0-9-]{1,63}\.)+test$`)
	nonNameRe   = regexp.MustCompile(`[^a-z0-9-]+`)
	multiDashRe = regexp.MustCompile(`-{2,}`)
)

// NormalizeName reduz s a [a-z0-9-]: minúsculas, qualquer outra sequência vira
// "-", hifens repetidos são colapsados e hifens nas pontas removidos.
func NormalizeName(s string) string {
	s = strings.ToLower(s)
	s = nonNameRe.ReplaceAllString(s, "-")
	s = multiDashRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// ApplyDefaults preenche campos ausentes a partir do diretório do projeto:
// Name = basename normalizado; Domain = NormalizeName(Name)+".test";
// Docroot = "public" se root/public existir, senão "" (raiz).
func ApplyDefaults(m *Manifest, root string) {
	if m.Name == "" {
		m.Name = NormalizeName(filepath.Base(root))
	}
	if m.Domain == "" {
		m.Domain = NormalizeName(m.Name) + ".test"
	}
	if m.Docroot == "" {
		m.Docroot = docrootPadrao(root)
	}
}

// docrootPadrao escolhe entre a raiz do projeto e public/.
//
// A presença da pasta public/ não basta: muito projeto legado guarda só
// assets ali e serve pela raiz, onde o index.php e o .htaccess fazem o
// roteamento. Apontar o docroot para public/ nesse caso entrega um 404 com
// tudo o mais correto. Então public/ só vence quando tem o index e a raiz não
// tem — que é o caso do Laravel e dos frameworks que seguem o mesmo layout.
func docrootPadrao(root string) string {
	if temIndice(root) {
		return ""
	}
	if temIndice(filepath.Join(root, "public")) {
		return "public"
	}
	return ""
}

// temIndice responde se o diretório tem um arquivo que o servidor entregaria
// como raiz. A lista acompanha o DirectoryIndex dos templates de vhost.
func temIndice(dir string) bool {
	for _, nome := range []string{"index.php", "index.html", "index.htm"} {
		if st, err := os.Stat(filepath.Join(dir, nome)); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

// Validate garante o mínimo para gerar um vhost: nome utilizável, domínio .test,
// PHP no formato major.minor e processes bem formados.
func Validate(m Manifest) error {
	if m.Name == "" {
		return fmt.Errorf("manifesto: name vazio")
	}
	if NormalizeName(m.Name) == "" {
		return fmt.Errorf("manifesto: name %q não contém caracteres válidos [a-z0-9-]", m.Name)
	}
	if !strings.HasSuffix(m.Domain, ".test") || len(m.Domain) <= len(".test") {
		return fmt.Errorf("manifesto: domain %q deve terminar em .test", m.Domain)
	}
	// O domínio vai cru para ServerName/server_name, mkcert e o bloco do
	// hosts gravado pelo helper elevado: espaço ou quebra de linha viraria
	// argumento a mais ou uma entrada de hosts arbitrária.
	if !domainRe.MatchString(m.Domain) {
		return fmt.Errorf("manifesto: domain %q não é um nome de host válido (rótulos [a-z0-9-] de até 63 caracteres)", m.Domain)
	}
	// Docroot absoluto ou com ".." tiraria o DocumentRoot de dentro do
	// projeto — e o vhost escuta em todas as interfaces.
	if d := filepath.ToSlash(m.Docroot); d != "" {
		clean := path.Clean(d)
		if filepath.IsAbs(m.Docroot) || filepath.VolumeName(m.Docroot) != "" || strings.HasPrefix(d, "/") ||
			clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("manifesto: docroot %q deve ser relativo e ficar dentro do projeto", m.Docroot)
		}
	}
	if m.PHP != "" && !phpMajorRe.MatchString(m.PHP) {
		return fmt.Errorf("manifesto: php %q deve ter o formato major.minor (ex.: \"8.1\")", m.PHP)
	}
	for name, cmd := range m.Processes {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("manifesto: processes contém chave vazia")
		}
		if strings.TrimSpace(cmd) == "" {
			return fmt.Errorf("manifesto: processes.%s tem comando vazio", name)
		}
	}
	return nil
}
