// Package version guarda a versão do HyPHP que o próprio binário declara.
package version

// Current é a versão comparada pelo auto-update com o manifesto publicado.
//
// Existe em mais três lugares, que alimentam o recurso de versão do exe e o
// instalador: `info.version` em build/config.yml, build/windows/info.json e
// INFO_PRODUCTVERSION em build/windows/nsis/wails_tools.nsh. O
// cmd/hyphp-release recusa publicar se divergirem, porque um binário
// que se declara mais velho do que é acha sempre uma versão "nova" no
// manifesto e entra em loop de update.
const Current = "3.0.0"
