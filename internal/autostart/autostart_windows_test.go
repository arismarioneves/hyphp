package autostart

import "testing"

// Usa o registro real do usuário: é HKCU, não precisa de elevação, e o valor é
// removido no fim. Testar contra um fake não provaria nada — o que pode dar
// errado aqui é exatamente a interação com o registro.
func TestApplyGravaERemove(t *testing.T) {
	t.Cleanup(func() { _ = Apply(false) })

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
	if err := Apply(false); err != nil {
		t.Fatalf("Apply(false) com entrada ausente: %v", err)
	}
}
