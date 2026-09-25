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
	"strconv"
	"strings"
)

// Faixa é um intervalo de versões [Min, Max). Max vazio significa "sem teto".
type Faixa struct {
	Min string // inclusivo, "7.2.5"
	Max string // exclusivo, "8.3"; vazio = sem limite
}

// phpParaPhpMyAdmin mapeia a série MAIOR.MENOR do phpMyAdmin para a faixa de
// PHP que ela aceita. Fonte: requisitos publicados pelo próprio projeto.
//
// O branch 5.2 é o último que roda em PHP 7; o 6.x exige 8.2+. Quem tem só PHP
// 8.4 instalado não consegue usar o 5.2, e é exatamente esse caso que precisa
// virar aviso em vez de erro de execução.
var phpParaPhpMyAdmin = map[string]Faixa{
	"5.2": {Min: "7.2.5", Max: "8.3"},
	"6.0": {Min: "8.2"},
	"6.1": {Min: "8.2"},
}

// PHPParaPhpMyAdmin devolve a faixa de PHP exigida por uma versão do
// phpMyAdmin. Versão desconhecida devolve ok=false: é melhor não afirmar nada
// do que chutar uma faixa e bloquear uma combinação que funciona.
func PHPParaPhpMyAdmin(versao string) (Faixa, bool) {
	f, ok := phpParaPhpMyAdmin[serie(versao)]
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
		if melhor == "" || compara(m, melhor) > 0 {
			melhor = m
		}
	}
	return melhor
}

// Contem responde se a versão cai na faixa. A comparação é numérica por
// componente: "8.10" é maior que "8.9", o que a ordem lexicográfica erra.
func (f Faixa) Contem(v string) bool {
	if f.Min != "" && compara(v, f.Min) < 0 {
		return false
	}
	if f.Max != "" && compara(v, f.Max) >= 0 {
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

// serie reduz "5.2.3" a "5.2". As restrições valem por branch, não por patch.
func serie(v string) string {
	p := strings.SplitN(v, ".", 3)
	if len(p) < 2 {
		return v
	}
	return p[0] + "." + p[1]
}

// compara devolve -1, 0 ou 1 comparando componente a componente. Componente
// ausente conta como zero, para "8.1" e "8.1.0" serem equivalentes.
func compara(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		na, nb := componente(pa, i), componente(pb, i)
		if na != nb {
			if na < nb {
				return -1
			}
			return 1
		}
	}
	return 0
}

func componente(p []string, i int) int {
	if i >= len(p) {
		return 0
	}
	n, err := strconv.Atoi(p[i])
	if err != nil {
		return 0
	}
	return n
}
