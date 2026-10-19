package sysinfo

import "testing"

func TestNomeDoSistemaVemDoOSRelease(t *testing.T) {
	casos := []struct {
		nome, raw, quer string
	}{
		{"PRETTY_NAME entre aspas", "NAME=\"Ubuntu\"\nVERSION_ID=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n", "Ubuntu 24.04.1 LTS"},
		{"sem PRETTY_NAME", "NAME=\"Linux Mint\"\nVERSION_ID=\"22\"\n", "Linux Mint 22"},
		{"sem aspas e com comentário", "# gerado\nNAME=Debian\nVERSION_ID=13\n", "Debian 13"},
		{"aspas simples", "PRETTY_NAME='Debian GNU/Linux 13 (trixie)'\n", "Debian GNU/Linux 13 (trixie)"},
		{"vazio", "", "Linux"},
	}
	for _, c := range casos {
		if got := osRelease(c.raw); got != c.quer {
			t.Errorf("%s: osRelease = %q; quero %q", c.nome, got, c.quer)
		}
	}
}
