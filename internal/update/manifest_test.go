package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

const shaValido = "3f1c0000000000000000000000000000000000000000000000000000000000ab"

func releaseValido() Release {
	return Release{
		Version: "1.0.0",
		Date:    "2026-09-26",
		Notes:   []string{"Primeira versão."},
		WindowsAMD64: &Artifact{
			Path:   "1.0.0/hyphp-1.0.0-windows-amd64-setup.exe",
			Size:   9613704,
			SHA256: shaValido,
		},
	}
}

func chaves(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

// A assinatura é o que impede quem comprometer a hospedagem de trocar o
// instalador e o sha256 juntos; qualquer byte fora do lugar tem de derrubá-la.
func TestVerify(t *testing.T) {
	pub, priv := chaves(t)
	outraPub, _ := chaves(t)
	body := []byte(`{"schema":1,"version":"1.0.0"}`)
	sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, body)))

	if err := Verify(pub, body, sig); err != nil {
		t.Errorf("assinatura válida recusada: %v", err)
	}
	// O .sig publicado termina em \n; editores e FTP acrescentam espaços.
	if err := Verify(pub, body, append(append([]byte(" "), sig...), '\r', '\n')); err != nil {
		t.Errorf("espaços ao redor deveriam ser ignorados: %v", err)
	}
	adulterado := append([]byte(nil), body...)
	adulterado[len(adulterado)-2] = '1'
	if err := Verify(pub, adulterado, sig); err == nil {
		t.Error("corpo adulterado aceito")
	}
	if err := Verify(outraPub, body, sig); err == nil {
		t.Error("chave errada aceita")
	}
	if err := Verify(pub, body, []byte("não é base64")); err == nil {
		t.Error("assinatura ilegível aceita")
	}
}

func TestParseLatest(t *testing.T) {
	ok := `{"schema":1,"version":"1.2.3","date":"2026-09-26","notes":[],` +
		`"windows_amd64":{"path":"1.2.3/x.exe","size":10,"sha256":"` + shaValido + `"}}`
	l, err := ParseLatest([]byte(ok))
	if err != nil {
		t.Fatalf("manifesto válido recusado: %v", err)
	}
	if l.Version != "1.2.3" || l.WindowsAMD64 == nil || l.WindowsAMD64.Size != 10 {
		t.Errorf("campos lidos errado: %+v", l)
	}

	// Um schema desconhecido pode ter mudado o sentido de qualquer campo: o
	// app antigo não deve tentar adivinhar.
	schema2 := strings.Replace(ok, `"schema":1`, `"schema":2`, 1)
	if _, err := ParseLatest([]byte(schema2)); err == nil {
		t.Error("schema 2 aceito")
	}
	if _, err := ParseLatest([]byte(`{`)); err == nil {
		t.Error("JSON quebrado aceito")
	}
}

func TestReleaseValidate(t *testing.T) {
	if err := releaseValido().Validate(); err != nil {
		t.Fatalf("release válido recusado: %v", err)
	}
	casos := map[string]func(*Release){
		"versão com duas partes":     func(r *Release) { r.Version = "1.0" },
		"versão com pré-release":     func(r *Release) { r.Version = "1.0.0-beta" },
		"versão com zero à esquerda": func(r *Release) { r.Version = "1.01.0" },
		"data em outro formato":      func(r *Release) { r.Date = "26/09/2026" },
		"sem artefato windows":       func(r *Release) { r.WindowsAMD64 = nil },
		"tamanho zero":               func(r *Release) { r.WindowsAMD64.Size = 0 },
		"sha curto":                  func(r *Release) { r.WindowsAMD64.SHA256 = shaValido[1:] },
		"sha maiúsculo":              func(r *Release) { r.WindowsAMD64.SHA256 = strings.ToUpper(shaValido) },
		"path absoluto":              func(r *Release) { r.WindowsAMD64.Path = "/1.0.0/x.exe" },
		"path com ..":                func(r *Release) { r.WindowsAMD64.Path = "../x.exe" },
		"path com esquema":           func(r *Release) { r.WindowsAMD64.Path = "https://outro.host/x.exe" },
		"path com barra invertida":   func(r *Release) { r.WindowsAMD64.Path = `1.0.0\x.exe` },
		"path vazio":                 func(r *Release) { r.WindowsAMD64.Path = "" },
	}
	for nome, estraga := range casos {
		r := releaseValido()
		a := *r.WindowsAMD64
		r.WindowsAMD64 = &a
		estraga(&r)
		if err := r.Validate(); err == nil {
			t.Errorf("%s: aceito", nome)
		}
	}
}

// Comparação lexicográfica diria que 1.0.10 < 1.0.9, e a 1.0.10 nunca seria
// oferecida a quem está na 1.0.9.
func TestCompare(t *testing.T) {
	casos := []struct {
		a, b string
		quer int
	}{
		{"1.0.10", "1.0.9", 1},
		{"1.2.0", "1.2.0", 0},
		{"0.9.9", "1.0.0", -1},
		{"2.0.0", "1.99.99", 1},
	}
	for _, c := range casos {
		got, err := Compare(c.a, c.b)
		if err != nil || got != c.quer {
			t.Errorf("Compare(%s, %s) = %d, %v; quer %d", c.a, c.b, got, err, c.quer)
		}
	}
	if _, err := Compare("1.0", "1.0.0"); err == nil {
		t.Error("versão inválida comparada sem erro")
	}
}
