// Package update verifica, baixa e aplica versões novas do HyPHP.
//
// O formato publicado é o de C:\DEV\hyphp-web\docs\contrato-releases.md, que é
// autoritativo: toda versão instalada fica para sempre lendo o mesmo endereço
// e esperando o mesmo formato, então campo existente nunca muda de sentido.
package update

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SchemaVersion é o único schema que este binário entende.
const SchemaVersion = 1

// Artifact é um arquivo publicado para uma plataforma.
type Artifact struct {
	Path   string `json:"path"` // relativo à pasta releases/
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Release é uma versão publicada.
type Release struct {
	Version      string    `json:"version"`
	Date         string    `json:"date"`
	Notes        []string  `json:"notes"`
	WindowsAMD64 *Artifact `json:"windows_amd64,omitempty"`
}

// Latest é o latest.json: um Release com o schema ao lado, em JSON plano.
type Latest struct {
	Schema int `json:"schema"`
	Release
}

// Index é o index.json: o histórico, da versão mais nova para a mais antiga.
type Index struct {
	Schema   int       `json:"schema"`
	Releases []Release `json:"releases"`
}

var (
	versionRe = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	sha256Re  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Verify confere a assinatura ed25519 (base64 padrão) dos bytes exatos do
// manifesto. Espaços ao redor da assinatura são ignorados: o .sig publicado
// termina em quebra de linha, e FTP/editores costumam mexer nisso.
func Verify(pub ed25519.PublicKey, body, sig []byte) error {
	raw, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(sig)))
	if err != nil {
		return fmt.Errorf("update: assinatura ilegível: %w", err)
	}
	if !ed25519.Verify(pub, body, raw) {
		return errors.New("update: assinatura do manifesto inválida")
	}
	return nil
}

// ParseLatest decodifica e valida o latest.json. Não confere a assinatura:
// quem chama faz Verify antes, sobre os mesmos bytes.
func ParseLatest(body []byte) (Latest, error) {
	var l Latest
	if err := json.Unmarshal(body, &l); err != nil {
		return Latest{}, fmt.Errorf("update: manifesto ilegível: %w", err)
	}
	// Um schema desconhecido pode ter mudado o sentido de qualquer campo.
	if l.Schema != SchemaVersion {
		return Latest{}, fmt.Errorf("update: schema %d não suportado (este binário entende %d)", l.Schema, SchemaVersion)
	}
	if err := l.Validate(); err != nil {
		return Latest{}, err
	}
	return l, nil
}

// Validate aplica as regras do contrato. É também o que o hyphp-release roda
// antes de publicar, para que o site nunca sirva algo que o app recusaria.
func (r Release) Validate() error {
	if !versionRe.MatchString(r.Version) {
		return fmt.Errorf("update: versão %q fora do formato MAIOR.MENOR.CORREÇÃO", r.Version)
	}
	if _, err := time.Parse(time.DateOnly, r.Date); err != nil {
		return fmt.Errorf("update: data %q fora do formato AAAA-MM-DD", r.Date)
	}
	a := r.WindowsAMD64
	if a == nil {
		return errors.New("update: versão sem instalador windows_amd64")
	}
	if a.Size <= 0 {
		return fmt.Errorf("update: tamanho %d inválido", a.Size)
	}
	if !sha256Re.MatchString(a.SHA256) {
		return fmt.Errorf("update: sha256 %q não tem 64 hex minúsculos", a.SHA256)
	}
	return validPath(a.Path)
}

// validPath exige caminho relativo à pasta releases/. Absoluto ou com esquema
// faria o app baixar de outro lugar; ".." escaparia da pasta; "\" é o
// separador do Windows e não existe em URL.
func validPath(p string) error {
	switch {
	case p == "":
		return errors.New("update: path vazio")
	case strings.HasPrefix(p, "/"), strings.Contains(p, ":"), strings.Contains(p, `\`):
		return fmt.Errorf("update: path %q não é relativo à pasta releases/", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("update: path %q tem segmento inválido", p)
		}
	}
	return nil
}

// Compare devolve -1, 0 ou 1 comparando numericamente parte a parte. A ordem
// lexicográfica diria que 1.0.10 < 1.0.9.
func Compare(a, b string) (int, error) {
	pa, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	pb, err := parseVersion(b)
	if err != nil {
		return 0, err
	}
	for i := range pa {
		switch {
		case pa[i] > pb[i]:
			return 1, nil
		case pa[i] < pb[i]:
			return -1, nil
		}
	}
	return 0, nil
}

func parseVersion(v string) ([3]int, error) {
	m := versionRe.FindStringSubmatch(v)
	if m == nil {
		return [3]int{}, fmt.Errorf("update: versão %q fora do formato MAIOR.MENOR.CORREÇÃO", v)
	}
	var out [3]int
	for i := range out {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return [3]int{}, fmt.Errorf("update: versão %q: %w", v, err)
		}
		out[i] = n
	}
	return out, nil
}
