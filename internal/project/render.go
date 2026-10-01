package project

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"hyphp/internal/i18n"
)

// SiteURL é a página do HyPHP, citada no cabeçalho de todo hyphp.yaml gerado.
const SiteURL = "https://ae8.com.br/hyphp/"

// Render produz o conteúdo do hyphp.yaml.
//
// Não é yaml.Marshal direto por dois motivos. O arquivo vai para o repositório
// do projeto e é lido por quem clona: ele precisa dizer o que aceita, sem
// obrigar ninguém a abrir documentação. E os campos ausentes precisam aparecer
// como exemplo comentado — descobrir que existe `processes:` só lendo o código
// do HyPHP não é aceitável.
//
// Os valores comentados são derivados do próprio projeto (o domínio real, o
// nome real), para poderem ser descomentados e funcionarem.
func Render(m Manifest) []byte {
	var b strings.Builder
	// O link vai no topo porque o arquivo viaja com o repositório: quem clona
	// um projeto e não conhece o HyPHP precisa saber de onde ele vem.
	b.WriteString(i18n.T("yaml.header", SiteURL) + "\n")
	b.WriteString(i18n.T("yaml.headerSub") + "\n\n")

	// Valores do usuário passam por yamlEscalar: "loja: v2" ou "web #x" soltos
	// não voltariam iguais no Load, e o projeto sumiria.
	campo(&b, "name", yamlEscalar(m.Name), i18n.T("yaml.name"))
	campo(&b, "domain", yamlEscalar(m.Domain), i18n.T("yaml.domain"))
	// php sempre com aspas: 8.10 sem aspas vira o número 8.1 e a série muda
	// silenciosamente para outra versão.
	campo(&b, "php", quoted(m.PHP), i18n.T("yaml.php"))

	if m.Docroot != "" {
		campo(&b, "docroot", yamlEscalar(m.Docroot), i18n.T("yaml.docroot"))
	}
	if m.Wildcard {
		campo(&b, "wildcard", "true", i18n.T("yaml.wildcard", m.Domain))
	}
	if m.Database != "" {
		campo(&b, "database", yamlEscalar(m.Database), i18n.T("yaml.database"))
	}
	if len(m.Extensions) > 0 {
		b.WriteString("\n" + i18n.T("yaml.extensions") + "\n")
		lista(&b, "extensions", m.Extensions)
	}
	if len(m.Processes) > 0 {
		b.WriteString("\n" + i18n.T("yaml.processes") + "\n")
		mapa(&b, "processes", m.Processes)
	}

	ausentes(&b, m)
	return []byte(b.String())
}

// ausentes escreve, comentado, o que o projeto não usa. É a parte que torna o
// arquivo descobrível: sem ela o usuário não tem como saber que dá para
// declarar um worker ou um banco sem sair da documentação.
func ausentes(b *strings.Builder, m Manifest) {
	var linhas []string
	if m.Docroot == "" {
		linhas = append(linhas, comentado("docroot: public", i18n.T("yaml.docrootHint")))
	}
	if !m.Wildcard {
		linhas = append(linhas, comentado("wildcard: true", i18n.T("yaml.wildcardHint", m.Domain)))
	}
	if m.Database == "" {
		linhas = append(linhas, comentado("database: "+NormalizeName(m.Name), i18n.T("yaml.databaseHint")))
	}
	if len(m.Extensions) == 0 {
		linhas = append(linhas,
			comentado("extensions:", i18n.T("yaml.extensionsHint")),
			"#   - redis",
			"#   - imagick")
	}
	if len(m.Processes) == 0 {
		linhas = append(linhas,
			comentado("processes:", i18n.T("yaml.processesHint")),
			"#   queue: php artisan queue:work",
			"#   vite: npm run dev")
	}
	if len(linhas) == 0 {
		return
	}
	b.WriteString("\n" + i18n.T("yaml.uncomment") + "\n")
	b.WriteString(strings.Join(linhas, "\n"))
	b.WriteString("\n")
}

// comentado alinha a coluna do comentário para o bloco ficar legível.
func comentado(decl, nota string) string {
	return fmt.Sprintf("# %-28s # %s", decl, nota)
}

func campo(b *strings.Builder, chave, valor, comentario string) {
	if valor == "" {
		return
	}
	fmt.Fprintf(b, "%-10s %-22s # %s\n", chave+":", valor, comentario)
}

func lista(b *strings.Builder, chave string, valores []string) {
	fmt.Fprintf(b, "%s:\n", chave)
	for _, v := range valores {
		fmt.Fprintf(b, "  - %s\n", yamlEscalar(v))
	}
}

func mapa(b *strings.Builder, chave string, valores map[string]string) {
	fmt.Fprintf(b, "%s:\n", chave)
	nomes := make([]string, 0, len(valores))
	for k := range valores {
		nomes = append(nomes, k)
	}
	// Ordem estável: sem isso o arquivo muda de forma a cada gravação e polui
	// o diff de quem versiona o projeto.
	sort.Strings(nomes)
	for _, k := range nomes {
		fmt.Fprintf(b, "  %s: %s\n", k, yamlEscalar(valores[k]))
	}
}

// yamlEscalar cita o valor quando ele não é seguro solto (dois-pontos, aspas,
// começo com caractere especial). Delega ao yaml.v3 para não reinventar as
// regras de quoting. Vazio continua vazio: é o sinal de campo ausente para
// campo, que então não escreve a linha.
func yamlEscalar(v string) string {
	if v == "" {
		return ""
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprintf("%q", v)
	}
	enc.Close()
	return strings.TrimSpace(buf.String())
}

func quoted(v string) string {
	if v == "" {
		return ""
	}
	return `"` + v + `"`
}
