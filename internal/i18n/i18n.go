// Package i18n traduz os textos que o Go mostra ao usuário: avisos da stack,
// menu do tray, títulos de diálogo, mensagens de erro dos services, a página
// padrão do web server e os comentários do hyphp.yaml.
//
// O idioma é um só para o processo inteiro (SetCurrent), porque o HyPHP é um
// app de desktop de um usuário: passar o idioma por parâmetro até cada aviso
// não compraria nada. A interface React tem o próprio catálogo
// (frontend/src/i18n); os dois seguem a mesma regra de escolha (Resolve).
//
// Para acrescentar um idioma: um arquivo messages_<código>.go com o mapa
// completo, registrado em catalogs e Supported. TestCatalogosCompletos falha
// enquanto faltar alguma chave.
package i18n

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// Lang é um código de idioma BCP 47 ("pt-BR", "en").
type Lang string

const (
	PT Lang = "pt-BR"
	EN Lang = "en"
)

// Supported lista os idiomas na ordem em que a interface os oferece.
var Supported = []Lang{PT, EN}

// catalogs liga cada idioma ao seu mapa chave → texto (formato de fmt).
var catalogs = map[Lang]map[string]string{
	PT: messagesPT,
	EN: messagesEN,
}

var current atomic.Value // Lang

func init() { current.Store(PT) }

// SetCurrent troca o idioma do processo. Idioma desconhecido vira EN.
func SetCurrent(l Lang) {
	if _, ok := catalogs[l]; !ok {
		l = EN
	}
	current.Store(l)
}

// Current devolve o idioma do processo.
func Current() Lang { return current.Load().(Lang) }

// Valid diz se pref é um valor aceito em state.Language: vazio (segue o
// Windows) ou um idioma suportado.
func Valid(pref string) bool {
	if pref == "" {
		return true
	}
	_, ok := catalogs[Lang(pref)]
	return ok
}

// Resolve escolhe o idioma: a preferência gravada, se houver, senão o idioma
// da interface do Windows. Português de qualquer região vira pt-BR; o resto
// fica em inglês, que alcança mais gente do que o português.
func Resolve(pref string) Lang {
	if pref != "" && Valid(pref) {
		return Lang(pref)
	}
	return fromSystem(systemLanguages())
}

// fromSystem aplica a regra de Resolve à lista de idiomas do Windows, do
// preferido para o menos preferido.
func fromSystem(langs []string) Lang {
	for _, l := range langs {
		tag := strings.ToLower(l)
		for _, s := range Supported {
			if tag == strings.ToLower(string(s)) {
				return s
			}
		}
		if strings.HasPrefix(tag, "pt") {
			return PT
		}
		if strings.HasPrefix(tag, "en") {
			return EN
		}
	}
	return EN
}

// T traduz key no idioma atual, formatando args como fmt.Sprintf. Chave sem
// tradução devolve a própria chave: aparece na tela e o teste de catálogo
// aponta a falta, em vez de sumir com o texto.
func T(key string, args ...any) string { return TIn(Current(), key, args...) }

// TIn é T num idioma específico (testes e textos gravados em arquivo).
func TIn(l Lang, key string, args ...any) string {
	msg, ok := catalogs[l][key]
	if !ok {
		msg, ok = catalogs[PT][key]
	}
	if !ok {
		return key
	}
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

// Errorf é fmt.Errorf com o formato vindo do catálogo. %w funciona como em
// fmt.Errorf, para quem compara com errors.Is.
func Errorf(key string, args ...any) error {
	msg, ok := catalogs[Current()][key]
	if !ok {
		msg, ok = catalogs[PT][key]
	}
	if !ok {
		msg = key
	}
	return fmt.Errorf(msg, args...)
}
