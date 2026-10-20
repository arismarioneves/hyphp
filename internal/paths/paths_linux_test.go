package paths

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A raiz segue o XDG_DATA_HOME e cai em ~/.local/share sem ele. Valor
// relativo não vale, pela especificação: conta como ausente.
func TestRaizPadraoNoLinuxSegueOXDG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(EnvRoot, "")

	dados := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dados)
	if got, want := resolveRoot(), filepath.Join(dados, "HyPHP"); got != want {
		t.Fatalf("com XDG_DATA_HOME: %q, want %q", got, want)
	}

	for _, v := range []string{"", "relativo/dados"} {
		t.Setenv("XDG_DATA_HOME", v)
		if got, want := resolveRoot(), filepath.Join(home, ".local", "share", "HyPHP"); got != want {
			t.Fatalf("XDG_DATA_HOME=%q: %q, want %q", v, got, want)
		}
	}

	outra := t.TempDir()
	t.Setenv(EnvRoot, outra)
	if got := resolveRoot(); got != outra {
		t.Fatalf("com HYPHP_ROOT: %q, want %q", got, outra)
	}
}

// pathDepois roda o profile.d num sh com o PATH dado e devolve o PATH final,
// como um shell de login o veria.
func pathDepois(t *testing.T, script, path string, vezes int) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "hyphp.sh")
	if err := os.WriteFile(f, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	fontes := strings.Repeat(". '"+f+"'; ", vezes)
	cmd := exec.Command("/bin/sh", "-c", fontes+`printf '%s' "$PATH"`)
	cmd.Env = []string{"PATH=" + path}
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// O dono da pasta ganha a cli e o php-bin no fim do PATH, uma vez só mesmo
// com o arquivo lido de novo (um shell de login dentro de outro); os outros
// usuários da máquina ficam com o PATH deles.
func TestProfileDSoAcrescentaAPastaDoDonoUmaVez(t *testing.T) {
	cli := "/home/joão silva/.local/share/HyPHP/cli"
	base := "/usr/local/bin:/usr/bin:/bin"
	dono := ProfileDContent(cli, os.Getuid())
	quer := base + ":" + cli + ":" + cli + "/php-bin"
	if got := pathDepois(t, dono, base, 2); got != quer {
		t.Fatalf("PATH do dono = %q, quer %q", got, quer)
	}
	outro := ProfileDContent(cli, os.Getuid()+1)
	if got := pathDepois(t, outro, base, 1); got != base {
		t.Fatalf("PATH de outro usuário = %q, quer %q", got, base)
	}
}
