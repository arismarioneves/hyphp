package runtime

import (
	"os"
	"path/filepath"
	"regexp"
)

// versaoPMA casa a versão anunciada no README do phpMyAdmin ("phpMyAdmin 5.2.3"
// ou a linha "Version 5.2.3" do cabeçalho, conforme a release).
var versaoPMA = regexp.MustCompile(`(?i)(?:phpmyadmin|version)[\s-]+(\d+\.\d+\.\d+)`)

// detectPhpMyAdmin identifica uma árvore do phpMyAdmin. Diferente dos outros
// detectores, não executa nada: é código PHP, não binário, e rodar `php -r` só
// para ler uma versão custaria um processo por varredura.
func detectPhpMyAdmin(dir string) (Installed, bool) {
	if _, err := os.Stat(filepath.Join(dir, "index.php")); err != nil {
		return Installed{}, false
	}
	raw, err := os.ReadFile(filepath.Join(dir, "README"))
	if err != nil {
		return Installed{}, false
	}
	m := versaoPMA.FindSubmatch(raw)
	if m == nil {
		return Installed{}, false
	}
	v := string(m[1])
	// Major igual a Version: não há seleção por série como no PHP — o Reconcile
	// usa a única instalação presente.
	return Installed{Kind: PhpMyAdmin, Version: v, Major: v, Dir: dir}, true
}
