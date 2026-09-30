package autostart

import "testing"

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
