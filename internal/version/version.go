// Package version guarda a versão do HyPHP que o próprio binário declara.
package version

// Current é a versão comparada pelo auto-update com o manifesto publicado.
//
// Muda junto com os arquivos listados no AGENTS.md (item Versão). O
// `go run ./cmd/hyphp-release -verificar` confere os que dão a versão do exe,
// do instalador, do Info.plist e do .deb, porque um binário que se declara
// mais velho do que é acha sempre uma versão "nova" no manifesto e entra em
// loop de update.
const Current = "5.0.0"
