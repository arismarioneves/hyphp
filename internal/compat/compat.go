// Package compat concentra as restrições de versão entre os componentes do
// ambiente.
//
// Existe para que a regra viva num lugar só e possa ser testada sem subir nada.
// Espalhada pelo Reconcile, ela vira condicional escondida e o usuário descobre
// a incompatibilidade quando a ferramenta abre numa tela de erro.
//
// Só entram restrições verificadas. Apache e nginx NÃO restringem a série do
// PHP: com FastCGI o web server não carrega o PHP, apenas conversa por socket —
// qualquer combinação funciona, e inventar um aviso ali seria ruído. A conexão
// de PHP 7.x ao MySQL 8.0 também foi medida e funciona com o root sem senha que
// o HyPHP configura, então não há regra a declarar.
package compat

import (
	"fmt"

	"hyphp/internal/runtime"
)

// Faixa é um intervalo de versões [Min, Max). Max vazio significa "sem teto".
type Faixa struct {
	Min string // inclusivo, "7.2.5"
	Max string // exclusivo, "8.3"; vazio = sem limite
}

// phpParaPhpMyAdmin mapeia a série MAIOR.MENOR do phpMyAdmin para a faixa de
// PHP que ela aceita.
//
// Fonte: o `require.php` do composer.json do próprio pacote — "^7.2.5 || ^8.0"
// no 5.2.x, que é >= 7.2.5 e < 9. Ler a constraint declarada é o que evita
// inventar teto: a primeira versão desta tabela cortava em 8.3 por suposição,
// e o efeito era recusar o phpMyAdmin em quem tinha PHP 8.3+ instalado, com a
// combinação funcionando perfeitamente.
var phpParaPhpMyAdmin = map[string]Faixa{
	"5.2": {Min: "7.2.5", Max: "9"},
	"4.9": {Min: "5.5", Max: "8"},
}

// PHPParaPhpMyAdmin devolve a faixa de PHP exigida por uma versão do
// phpMyAdmin. Versão desconhecida devolve ok=false: é melhor não afirmar nada
// do que chutar uma faixa e bloquear uma combinação que funciona.
func PHPParaPhpMyAdmin(versao string) (Faixa, bool) {
	f, ok := phpParaPhpMyAdmin[runtime.MajorOf(versao)]
	return f, ok
}

// MelhorPHP escolhe a maior série dentro da faixa. Devolve "" quando nenhuma
// serve — o chamador transforma isso num aviso que nomeia o que falta.
func MelhorPHP(majors []string, f Faixa) string {
	melhor := ""
	for _, m := range majors {
		if !f.Contem(m) {
			continue
		}
		if melhor == "" || runtime.CompareVersions(m, melhor) > 0 {
			melhor = m
		}
	}
	return melhor
}

// Contem responde se a versão cai na faixa. A comparação é numérica por
// componente: "8.10" é maior que "8.9", o que a ordem lexicográfica erra.
func (f Faixa) Contem(v string) bool {
	if f.Min != "" && runtime.CompareVersions(v, f.Min) < 0 {
		return false
	}
	if f.Max != "" && runtime.CompareVersions(v, f.Max) >= 0 {
		return false
	}
	return true
}

// String descreve a faixa para mensagem de usuário.
func (f Faixa) String() string {
	switch {
	case f.Min != "" && f.Max != "":
		return fmt.Sprintf("PHP %s ou maior, e menor que %s", f.Min, f.Max)
	case f.Min != "":
		return fmt.Sprintf("PHP %s ou maior", f.Min)
	case f.Max != "":
		return fmt.Sprintf("PHP menor que %s", f.Max)
	}
	return "qualquer versão do PHP"
}
