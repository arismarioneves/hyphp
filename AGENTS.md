# Regras do projeto

Valem para quem contribui e para agentes de código.

## O projeto

- **App desktop:** Windows, macOS e Linux, em Go 1.26 com Wails v3 (`main.go`, `services/`, `internal/`) e interface em React com TypeScript (`frontend/`).
- **CLI:** `cmd/hyphp`.
- **Site:** `web/`, com regras próprias em [`web/README.pt-BR.md`](web/README.pt-BR.md).
- **Código de um SO só:** fica em arquivos `_windows.go`, `_darwin.go` e `_linux.go`; o que o Mac e o Linux fazem igual fica em `_unix.go`, com `//go:build darwin || linux` (o sufixo `_unix` sozinho não restringe nada). O comportamento de um SO só muda de propósito, nunca como efeito colateral de mudar o outro.

## Comentários no código

- Descreva o estado atual, não o histórico. O comentário diz o que o código faz e por quê agora — nunca o que era antes, o que mudou ou por que deixou de ser assim. Histórico é responsabilidade do git.
- Explique o porquê; o que o código faz, ele mesmo já diz. Não comente o óbvio.
- Em português.
- O repositório é público: comentários, mensagens de commit e documentação não citam segredos, detalhes de infraestrutura nem planos internos.

## Textos do app

- Todo texto que o usuário vê vem dos catálogos de idioma, em português e inglês, dizendo a mesma coisa:
  - Go: `internal/i18n/messages_pt.go` e `messages_en.go`, usados por `i18n.T` e `i18n.Errorf`;
  - frontend: `frontend/src/i18n/locales/pt-BR` e `en`.
- Erro interno leva o pacote e o contexto: `fmt.Errorf("stack: gravar %s: %w", caminho, err)`.

## Go

- `gofmt` nos arquivos alterados.
- `for i := range n`; `net.JoinHostPort` para montar endereço com porta.

## TypeScript

- Sem `any`.
- Sem função de uma linha usada uma vez só para dar nome a uma expressão.

## Testes

- Nomes em português, dizendo o comportamento testado.
- Testam o que um usuário do código percebe: resultado, limites, erros, invariantes e precedência. Não testam fiação, cópia de valores, texto do código nem tautologia.
- Determinísticos e isolados: `t.TempDir()`, `t.Setenv` e servidores locais, nunca a rede de verdade.
- Um golden só muda quando a saída deve mudar; confira o diff antes de aceitar.

## Antes de abrir PR

```sh
go build . ./cmd/... ./services/... ./internal/...
go vet . ./cmd/... ./services/... ./internal/...
go run honnef.co/go/tools/cmd/staticcheck@2026.2.1 . ./cmd/... ./services/... ./internal/...
go test . ./cmd/... ./services/... ./internal/...
npx --prefix frontend tsc --noEmit -p frontend
npm --prefix frontend test
```

- **No Windows:** confira também o código do macOS e do Linux. A tag `server` tira o cgo de janela do Wails e deixa o `main.go` e o `services/` compilarem de fora do sistema; o build de verdade de cada um roda no CI.

  ```sh
  GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go vet -tags server . ./cmd/... ./services/... ./internal/...
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet -tags server . ./cmd/... ./services/... ./internal/...
  ```

- **Site:** `node .github/scripts/site.mjs conferir`.
- **Scripts de shell:** passam no `shellcheck`.

## Commits

- Em português, sem acento, começando pela área: `catalogo: ...`, `fix(pkgmgr): ...`, `site: ...`.
- Um assunto por commit.

## Versão, catálogo e publicação

- **Versão:** muda junto em `internal/version/version.go`, `build/config.yml` (`info.version`), `build/windows/info.json`, `build/windows/nsis/wails_tools.nsh`, `build/darwin/Info.plist`, `build/darwin/Info.dev.plist`, `build/linux/nfpm/nfpm.yaml` (`version`) e `cmd/hyphp-helper/hyphp-helper.exe.manifest`, com as notas em `release/notas/<versão>.json` (pt e en). `go run ./cmd/hyphp-release -verificar` confere. Não rode `common:update:build-assets`.
- **Catálogo:** em `internal/pkgmgr/catalog.json`, link novo de pacote vai com o sha256 conferido no arquivo baixado e com o `serial` maior.
- **Publicação:** push, PR, tag, release e disparo de workflow só a pedido do mantenedor.
