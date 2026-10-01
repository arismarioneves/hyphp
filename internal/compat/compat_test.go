package compat

import "testing"

// A faixa vem do `require.php` do pacote ("^7.2.5 || ^8.0"), não de suposição:
// a primeira versão desta tabela cortava em 8.3 e recusava o phpMyAdmin em
// quem tinha PHP 8.3+, com a combinação funcionando.
func TestPhpMyAdmin52AceitaTodoPHP8(t *testing.T) {
	f, ok := PHPParaPhpMyAdmin("5.2.3")
	if !ok {
		t.Fatal("5.2.3 deveria ter faixa conhecida")
	}
	for _, v := range []string{"7.2.5", "7.4.33", "8.0", "8.1", "8.2", "8.3", "8.4.26", "8.5.11"} {
		if !f.Contem(v) {
			t.Errorf("PHP %s deveria servir ao phpMyAdmin 5.2", v)
		}
	}
	for _, v := range []string{"7.2.4", "7.1", "5.6", "9.0"} {
		if f.Contem(v) {
			t.Errorf("PHP %s NÃO deveria servir ao phpMyAdmin 5.2", v)
		}
	}
}

// Versão fora da tabela não afirma nada: bloquear uma combinação que pode
// funcionar é pior do que ficar calado.
func TestVersaoDesconhecidaNaoAfirmaNada(t *testing.T) {
	if _, ok := PHPParaPhpMyAdmin("9.9.9"); ok {
		t.Error("versão desconhecida devolveu faixa")
	}
}

func TestMelhorPHPEscolheAMaiorDaFaixa(t *testing.T) {
	f := Faixa{Min: "7.2.5", Max: "8.3"}
	casos := []struct {
		nome      string
		instalado []string
		quer      string
	}{
		{"pega a maior compatível", []string{"7.4", "8.1", "8.4"}, "8.1"},
		{"ignora as fora da faixa", []string{"8.4", "8.5"}, ""},
		{"nada instalado", nil, ""},
		// 8.10 > 8.9 é numérico; a ordem lexicográfica escolheria 8.9 e o
		// usuário rodaria numa série mais velha sem entender por quê.
		{"compara numérico, não texto", []string{"8.9", "8.10"}, ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := MelhorPHP(c.instalado, f); got != c.quer {
				t.Errorf("MelhorPHP(%v) = %q, quero %q", c.instalado, got, c.quer)
			}
		})
	}
}

// A descrição vai para a tela; faixa sem teto não pode falar de teto.
func TestFaixaDescreveParaOUsuario(t *testing.T) {
	if got := (Faixa{Min: "7.2.5", Max: "8.3"}).String(); got != "PHP 7.2.5 ou maior, e menor que 8.3" {
		t.Errorf("got %q", got)
	}
	if got := (Faixa{Min: "8.2"}).String(); got != "PHP 8.2 ou maior" {
		t.Errorf("got %q", got)
	}
}
