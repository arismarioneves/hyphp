package project

import "testing"

// Letra de unidade só é caminho absoluto no Windows; no macOS "C:/x" é uma
// pasta "C:" dentro do projeto.
func TestValidateRecusaDocrootComLetraDeUnidade(t *testing.T) {
	m := Manifest{Name: "a", Domain: "a.test", Docroot: "C:/x"}
	if err := Validate(m); err == nil {
		t.Fatalf("Validate(%+v) aceitou docroot com letra de unidade", m)
	}
}
