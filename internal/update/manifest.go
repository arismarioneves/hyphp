// Package update verifica, baixa e aplica versões novas do HyPHP.
//
// Cada versão é uma release do GitHub. O app lê dois assets dela:
// latest.json (um Latest, com o instalador do Windows e o dmg do Mac) e
// latest.json.sig (ed25519 dos bytes exatos do latest.json, em base64). Toda
// versão instalada fica para sempre lendo o mesmo endereço e esperando o
// mesmo formato, então campo existente nunca muda de sentido.
package update

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"hyphp/internal/runtime"
)

// SchemaVersion é o único schema que este binário entende.
const SchemaVersion = 1

// Artifact é um arquivo publicado para uma plataforma.
type Artifact struct {
	Path   string `json:"path"` // relativo à pasta do manifesto; na release do GitHub, o nome do asset
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Release é uma versão publicada.
type Release struct {
	Version string `json:"version"`
	Date    string `json:"date"`
	// Notes são as notas em português, o campo de sempre; NotesEN, as mesmas
	// notas em inglês, na mesma ordem. A 1.0.0 e a 2.0.0 saíram só em
	// português, e o omitempty mantém o registro delas como foi publicado.
	Notes        []string  `json:"notes"`
	NotesEN      []string  `json:"notes_en,omitempty"`
	WindowsAMD64 *Artifact `json:"windows_amd64,omitempty"`
	DarwinARM64  *Artifact `json:"darwin_arm64,omitempty"`
}

// Latest é o latest.json: um Release com o schema ao lado, em JSON plano.
type Latest struct {
	Schema int `json:"schema"`
	Release
}

// Index é o index.json do site: o histórico, da versão mais nova para a mais
// antiga. O app não lê este arquivo.
type Index struct {
	Schema int `json:"schema"`
	// GitHub é o "<dono>/<repo>" das releases. O download de cada versão é o
	// asset com o último segmento de Path, na tag v<versão>.
	GitHub   string    `json:"github,omitempty"`
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
	if err := validPath(a.Path); err != nil {
		return err
	}
	// darwin_arm64 é opcional no contrato (as releases até a 3.0.1 não têm),
	// mas se vier tem de ser tão confiável quanto o do Windows: é ele que o
	// Mac baixa.
	if d := r.DarwinARM64; d != nil {
		if d.Size <= 0 {
			return fmt.Errorf("update: tamanho darwin_arm64 %d inválido", d.Size)
		}
		if !sha256Re.MatchString(d.SHA256) {
			return fmt.Errorf("update: sha256 darwin_arm64 %q não tem 64 hex minúsculos", d.SHA256)
		}
		if err := validPath(d.Path); err != nil {
			return err
		}
	}
	return nil
}

// validPath exige caminho relativo à pasta do manifesto. Absoluto ou com
// esquema faria o app baixar de outro lugar; ".." escaparia da pasta; "\" é o
// separador do Windows e não existe em URL.
func validPath(p string) error {
	switch {
	case p == "":
		return errors.New("update: path vazio")
	case strings.HasPrefix(p, "/"), strings.Contains(p, ":"), strings.Contains(p, `\`):
		return fmt.Errorf("update: path %q não é relativo à pasta do manifesto", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("update: path %q tem segmento inválido", p)
		}
	}
	return nil
}

// Compare devolve -1, 0 ou 1 comparando numericamente parte a parte. A ordem
// lexicográfica diria que 1.0.10 < 1.0.9. O formato é validado aqui, estrito;
// a ordem é a mesma do resto do app.
func Compare(a, b string) (int, error) {
	for _, v := range []string{a, b} {
		if !versionRe.MatchString(v) {
			return 0, fmt.Errorf("update: versão %q fora do formato MAIOR.MENOR.CORREÇÃO", v)
		}
	}
	return runtime.CompareVersions(a, b), nil
}
