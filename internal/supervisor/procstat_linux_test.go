package supervisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// statComCampos monta uma linha do /proc/<pid>/stat com o nome dado e 49
// campos depois dele, trocando estado, pai, grupo e início.
func statComCampos(nome string, state string, ppid, pgid, start string) string {
	f := make([]string, 50)
	for i := range f {
		f[i] = "0"
	}
	f[0], f[1], f[2], f[19] = state, ppid, pgid, start
	return "123 (" + nome + ") " + strings.Join(f, " ") + "\n"
}

func TestParseStatComNomeComEspacoEParenteses(t *testing.T) {
	got, err := parseStat(statComCampos("php-fpm: master process (/x (y))", "S", "1", "123", "98765"))
	if err != nil {
		t.Fatal(err)
	}
	if want := (procStat{state: 'S', ppid: 1, pgid: 123, startTicks: 98765}); got != want {
		t.Fatalf("parseStat = %+v; quero %+v", got, want)
	}
}

// Linha cortada vira erro: valor zerado faria o pid parecer outro processo.
func TestParseStatRecusaLinhaCurta(t *testing.T) {
	for _, raw := range []string{"", "123 (sleep", "123 (sleep) S 1 123"} {
		if _, err := parseStat(raw); err == nil {
			t.Errorf("parseStat(%q) sem erro", raw)
		}
	}
}

func TestProcessoProprioNoProc(t *testing.T) {
	info := readProcInfo(os.Getpid())
	if !info.alive || info.startSec <= 0 || info.pgid <= 0 || info.ppid <= 0 {
		t.Fatalf("readProcInfo(self) = %+v", info)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.EvalSymlinks(exe); info.exe != want {
		t.Fatalf("exe = %q; quero %q", info.exe, want)
	}
	if again := readProcInfo(os.Getpid()); again.startSec != info.startSec {
		t.Fatalf("início mudou entre leituras: %d e %d", info.startSec, again.startSec)
	}
}

// Depois de um `brew upgrade` o /proc/<pid>/exe do binário antigo vem com
// " (deleted)": é outro executável, nunca casa por prefixo.
func TestExeApagadoNaoCasa(t *testing.T) {
	if sameExecutable("/a/php-fpm", "/a/php-fpm (deleted)") {
		t.Fatal("exe apagado casou com o registrado")
	}
}
