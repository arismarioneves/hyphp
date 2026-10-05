package render

import (
	"strings"
	"testing"
)

// O log fica em Application Support, com espaço: o error_log tem de sair
// entre aspas para o fpm não cortar o caminho.
func TestRenderFPMConfGolden(t *testing.T) {
	got := RenderFPMConf("8.3", 9004, 4, "/Users/dev/Library/Application Support/HyPHP/log")
	checkGolden(t, "darwin/php-fpm.conf.golden", got)
}

// O fpm do 7.2 recusa diretiva desconhecida ("unknown entry") e não sobe;
// decorate_workers_output só existe a partir do 7.3.
func TestRenderFPMConfSemDecorateNo72(t *testing.T) {
	got := string(RenderFPMConf("7.2", 9000, 1, "/log"))
	if strings.Contains(got, "decorate_workers_output") {
		t.Fatalf("7.2 não conhece decorate_workers_output:\n%s", got)
	}
	if !strings.Contains(got, "\ncatch_workers_output = yes\n") {
		t.Fatalf("catch_workers_output devia continuar:\n%s", got)
	}
}
