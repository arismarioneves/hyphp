package autostart

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// nomeDeTeste troca a entrada gravada por uma só do teste. O registro é o
// real do usuário, e a entrada "HyPHP" é a do app instalado: removê-la
// desligava o autostart de quem roda os testes na mesma máquina.
func nomeDeTeste(t *testing.T) {
	t.Helper()
	antes := valueName
	valueName = "HyPHP-teste-autostart"
	t.Cleanup(func() {
		_ = Apply(false)
		valueName = antes
	})
}

// Usa o registro real do usuário: é HKCU, não precisa de elevação, e o valor é
// removido no fim. Testar contra um fake não provaria nada — o que pode dar
// errado aqui é exatamente a interação com o registro.
func TestApplyGravaERemove(t *testing.T) {
	nomeDeTeste(t)

	if err := Apply(true); err != nil {
		t.Fatalf("Apply(true): %v", err)
	}
	on, err := Enabled()
	if err != nil {
		t.Fatalf("Enabled: %v", err)
	}
	if !on {
		t.Error("Enabled() = false depois de Apply(true)")
	}

	if err := Apply(false); err != nil {
		t.Fatalf("Apply(false): %v", err)
	}
	if on, err := Enabled(); err != nil || on {
		t.Errorf("Enabled() = %v, %v; quero false, nil", on, err)
	}
}

// Desligar quando já está desligado não pode virar erro: o Set das
// configurações chama Apply a cada gravação, inclusive quando nada mudou.
func TestApplyFalseIdempotente(t *testing.T) {
	nomeDeTeste(t)
	if err := Apply(false); err != nil {
		t.Fatalf("Apply(false) com entrada ausente: %v", err)
	}
}

// O Windows executa a entrada no login: ela tem de abrir o exe atual com a
// Flag, que é o que faz o app nascer só na bandeja quando o usuário pediu.
func TestEntradaDeLoginAbreOExeAtualComAFlag(t *testing.T) {
	nomeDeTeste(t)
	if err := Apply(true); err != nil {
		t.Fatalf("Apply(true): %v", err)
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	linha, _, err := k.GetStringValue(valueName)
	if err != nil {
		t.Fatal(err)
	}
	args, err := windows.DecomposeCommandLine(linha)
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	if len(args) == 0 || args[0] != exe {
		t.Fatalf("entrada %q não abre %q", linha, exe)
	}
	if !Hidden(args[1:], true) {
		t.Errorf("entrada %q sem a flag de login", linha)
	}
}
