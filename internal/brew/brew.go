// Package brew instala, remove e encontra runtimes do Homebrew no macOS. A
// tabela, a varredura de <prefix>/opt e o parser de saída são Go neutro,
// testados nos dois SOs; só a execução do brew (exec_darwin.go) é do Mac.
//
// O HyPHP nunca chama `brew link`/`brew unlink`, nunca usa `brew services` e
// nunca roda o instalador do Homebrew: só mostra InstallCommand.
package brew

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hyphp/internal/pkgmgr"
	"hyphp/internal/runtime"
)

// InstallCommand é o comando oficial de instalação do Homebrew, exibido para o
// usuário copiar. O HyPHP não o executa: pede senha de administrador e as
// Command Line Tools do Xcode.
const InstallCommand = `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`

// ErrBusy: o brew segura travas globais ("Another active Homebrew process"),
// então uma segunda operação simultânea falharia no meio; recusamos antes.
var ErrBusy = errors.New("brew: já existe uma operação do Homebrew em andamento")

// ErrNotFound: nenhum executável do brew nos lugares conhecidos.
var ErrNotFound = errors.New("brew: Homebrew não encontrado")

// Formula é uma entrada da tabela fixa de runtimes instaláveis pelo Homebrew.
type Formula struct {
	Name   string // nome de instalação: "shivammathur/php/php@8.3", "httpd", "mysql@8.4"
	Kind   runtime.Kind
	Series string   // rótulo antes de instalar: "8.3", "11.4", "" para httpd/nginx/mailpit/mkcert
	Opt    []string // pastas em <prefix>/opt a varrer: {"php@8.3"}; 8.5 → {"php@8.5", "php"}
}

// Short é o último segmento do nome: o que `brew uninstall` e <prefix>/opt usam.
func (f Formula) Short() string {
	return f.Name[strings.LastIndex(f.Name, "/")+1:]
}

// phpFormula monta a entrada de uma série do tap shivammathur/php. O nome
// completo faz o Homebrew 6+ confiar só nessa fórmula, sem `brew tap` nem
// `brew trust` do tap inteiro.
func phpFormula(series string, opt ...string) Formula {
	short := "php@" + series
	return Formula{Name: "shivammathur/php/" + short, Kind: runtime.PHP, Series: series, Opt: append([]string{short}, opt...)}
}

// simple é uma fórmula do homebrew-core cuja pasta opt/ tem o próprio nome.
func simple(name string, kind runtime.Kind, series string) Formula {
	return Formula{Name: name, Kind: kind, Series: series, Opt: []string{name}}
}

// Formulas é a tabela da spec §1, na ordem de exibição. Ficam de fora
// mysql@8.0 (descontinuado no Homebrew), mysql e mariadb sem versão (não são
// keg-only e conflitam entre si).
var Formulas = []Formula{
	// No tap, php@8.5 é alias de php: o keg pode aparecer como opt/php@8.5 e
	// como opt/php (a varredura deduplica pelo destino do link).
	phpFormula("8.5", "php"),
	phpFormula("8.4"),
	phpFormula("8.3"),
	phpFormula("8.2"),
	phpFormula("8.1"),
	phpFormula("8.0"),
	phpFormula("7.4"),
	phpFormula("7.2"),
	simple("httpd", runtime.Apache, ""),
	simple("nginx", runtime.Nginx, ""),
	simple("mysql@8.4", runtime.MySQL, "8.4"),
	simple("mariadb@11.8", runtime.MariaDB, "11.8"),
	simple("mariadb@11.4", runtime.MariaDB, "11.4"),
	simple("mariadb@10.11", runtime.MariaDB, "10.11"),
	simple("mailpit", runtime.Mailpit, ""),
	simple("mkcert", runtime.Mkcert, ""),
}

// Brew é uma instalação do Homebrew encontrada por Locate.
type Brew struct{ Exe, Prefix string }

// detect é variável para o teste da varredura trocar a execução dos binários
// por versões fixas (não há kegs de verdade fora do Mac).
var detect = runtime.Detect

// Scan detecta os kegs da tabela em <prefix>/opt, inclusive os instalados por
// fora do HyPHP (ex.: php@8.3 do homebrew-core). Dir fica no caminho opt/, que
// sobrevive a `brew upgrade` (o Cellar muda de pasta a cada versão). Pastas
// ausentes são ignoradas; falhas de detecção vão para o erro e a lista traz o
// que deu certo, como em runtime.Scan.
func (b Brew) Scan(ctx context.Context) ([]runtime.Installed, error) {
	var list []runtime.Installed
	var errs []error
	seen := map[string]bool{}
	for _, f := range Formulas {
		for _, name := range f.Opt {
			if err := ctx.Err(); err != nil {
				return list, err
			}
			dir := filepath.Join(b.Prefix, "opt", name)
			real, err := filepath.EvalSymlinks(dir)
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					errs = append(errs, fmt.Errorf("brew: resolver %s: %w", dir, err))
				}
				continue
			}
			// O mesmo keg por dois nomes (php e php@8.5) entra uma vez só.
			if seen[real] {
				continue
			}
			seen[real] = true
			inst, err := detect(f.Kind, dir)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			inst.Formula = f.Name
			inst.Prefix = b.Prefix
			list = append(list, inst)
		}
	}
	return list, errors.Join(errs...)
}

// phaseOf traduz uma linha do `brew install` na fase da barra de progresso.
// Linhas sem fase reconhecida devolvem "" e quem chama mantém a anterior.
func phaseOf(line string) string {
	switch {
	case strings.HasPrefix(line, "==> Fetching"), strings.HasPrefix(line, "==> Downloading"):
		return pkgmgr.PhaseDownload
	case strings.HasPrefix(line, "==> Pouring"), strings.HasPrefix(line, "==> Installing"):
		return pkgmgr.PhaseExtract
	}
	return ""
}
