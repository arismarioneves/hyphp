# Site do HyPHP

<p><a href="README.md">English</a> | <b>Português</b></p>

O site em `https://ae8.com.br/hyphp/` sai desta pasta. O design está aberto: layout, seções, tipografia, cores, ilustrações, movimento e a forma de apresentar o produto podem ser repensados. Mande a proposta por PR; o mantenedor valida o que muda na cara do site. As regras base abaixo não mudam.

## Regras base

1. **Sem build.** HTML, CSS e JavaScript escritos à mão, que abrem com duplo clique no `index.html`. Não entram framework, bundler, `package.json`, TypeScript, Sass, nem módulos ES. Em `file://` o navegador bloqueia módulos e `fetch()`; por isso os scripts são clássicos e os dados entram por `<script src>`.
2. **Nada carregado de fora.** Fontes, imagens e scripts ficam nesta pasta, com as licenças. Link para outro site pode; CDN não.
3. **Marca HyPHP.** O nome, o logo e o ícone (`assets/marca/`) ficam como são. Mudança nas cores principais da marca passa pela validação do mantenedor.
4. **Dois idiomas.**
   - Inglês é o padrão, e português a segunda opção.
   - Todo texto existe nos dois dicionários de `assets/textos.js`, dizendo a mesma coisa.
   - Chaves terminadas em `Html` levam marcação e são só strings fixas, nunca dado de fora.
5. **Só fatos.** Toda frase sobre o HyPHP tem prova no código deste repositório ([Fatos e provas](#fatos-e-provas)). Para texto com um fato novo, aponte a prova na PR. O site fala só do HyPHP e dos concorrentes da comparação, e o preço é só "Gratuito".
6. **Comparação** (`assets/comparacao.js`):
   - só fonte primária: site, documentação, repositório ou changelog oficial, ou texto do mantenedor no canal do projeto;
   - bloco de concorrente que afirma algo tem `fonte` e `em` (data da leitura);
   - sem fonte, o bloco não afirma nada (`suporte: null` com uma `nota`);
   - texto neutro, sem adjetivo.

   A forma de mostrar a comparação é livre.
7. **Download e histórico.**
   - Os dados vêm de `releases/releases.js` (`window.HYPHP_RELEASES`), que já existe no site publicado.
   - O botão Baixar e o histórico apontam para os assets das releases deste repositório (`https://github.com/arismarioneves/hyphp/releases/download/v<versão>/<asset>`).
   - As plataformas publicadas continuam com o que precisam: o instalador do Windows e o aviso do SmartScreen; o dmg, o comando do `install.sh` e o aviso do Gatekeeper no macOS.
8. **Arquivos de dados.**
   - Os textos ficam em `assets/textos.js` e a comparação em `assets/comparacao.js`, nesse formato.
   - As capturas ficam em `assets/telas/en` e `assets/telas/pt`, com os mesmos arquivos nas duas pastas, e só com projetos e dados de exemplo.
   - As pastas `releases/`, `catalogo/` e `docs/` não ficam em `web/`.
9. **Acessível e leve.** Com `prefers-reduced-motion`, nada se move. Sem rolagem lateral no celular (390×844). Console sem erro nem aviso.

## Livre para propor

Dentro das regras base, vale explorar e mandar para validação:
- layout, seções e ordem do conteúdo;
- cores, tipografia e fontes locais;
- ilustrações e animações;
- capturas novas;
- a galeria e a apresentação da comparação;
- o comportamento em cada tamanho de tela.

Hoje, de 1200×600 de janela para cima, tudo cabe sem rolagem. É uma referência do design atual, não uma regra.

## Ver localmente

Dê duplo clique em `web/index.html`. Para ver o botão Baixar e o histórico, baixe o histórico publicado (o git ignora essa cópia):

```sh
curl -o web/releases/releases.js https://ae8.com.br/hyphp/releases/releases.js
```

## Antes de abrir a PR

```sh
node .github/scripts/site.mjs conferir
```

Só precisa do Node. A conferência recusa:
- arquivos de build;
- as pastas `releases/`, `catalogo/` e `docs/`;
- recurso de fora ou que não existe;
- chave de tradução que falta num idioma;
- captura que só existe num idioma;
- bloco da comparação sem fonte.

Na PR, mande capturas da página em uma tela de computador e em 390×844, nos dois idiomas (`?lang=en` e `?lang=pt`), e diga o que mudou e por quê.

## Fatos e provas

| Texto do site | Prova no repositório |
|---|---|
| Windows 10 e 11 · x64 · macOS 15+ · Apple Silicon | instalador NSIS com WebView2 (`build/windows/nsis/project.nsi`); `LSMinimumSystemVersion` 15.0 em `build/darwin/Info.plist`; `build/darwin/install.sh` exige arm64 e macOS 15 |
| Aviso do Mac: sem notarização; se o macOS bloquear, Privacidade e Segurança › Abrir Mesmo Assim | assinatura ad hoc (`codesign --sign -`) em `build/darwin/Taskfile.yml`, sem notarização; o comando acima do aviso instala pelo `build/darwin/install.sh`, que baixa com o `curl`, e o `curl` não marca quarentena |
| Comando da aba macOS (`curl -fsSL …/install.sh \| sh`) | cabeçalho do `build/darwin/install.sh`; o asset `install.sh` de cada release sai do job `publicar` do `.github/workflows/build.yml` (`cmd/hyphp-release`, `stageAssets`) |
| Ambiente PHP declarado por projeto | versão de PHP no `hyphp.yaml` (`internal/project/manifest.go`, `Manifest.PHP`) |
| vhost no Apache ou no nginx, domínio .test, HTTPS local, MySQL ou MariaDB e processos de fila | `internal/webserver/*`, `internal/netcfg/hosts.go`, `internal/netcfg/mkcert.go`, `internal/stack/mysql.go` (`DBRuntime`), `internal/stack/desired.go` (`procSpecs`) |
| Sem Docker | processos nativos supervisionados (`internal/supervisor/process_windows.go`) |
| Inclui: PHP 7.2 a 8.5, Apache ou nginx, MySQL ou MariaDB, Mailpit, phpMyAdmin, HTTPS local, atualização automática | `internal/pkgmgr/catalog.json`, `internal/update/updater.go` |
| Inclui: php.ini por versão | diretivas do usuário por série em `state.PHPIni`, escritas no fim do php.ini por `internal/render/phpini.go` (`RenderPHPIni`); painel em `frontend/src/screens/PhpIniPanel.tsx` |
| Inclui: CLI | `cmd/hyphp`, `internal/cli`, `services/cli_service.go` |
| Telas › Stack, projetos, portas e avisos numa tela só; um pool de workers php-cgi por versão de PHP | `frontend/src/screens/Dashboard.tsx`; `internal/stack/desired.go` (specs `php:<série>:<n>`, `PoolSize`) |
| Telas › Projetos: versão de PHP, domínio, docroot, extensões, banco e processos | `internal/project/manifest.go` (`Manifest`), `internal/stack/desired.go` (`procSpecs`) |
| Telas › PHP 7.2 a 8.5 dos builds oficiais, conferidos por SHA-256; diretivas por versão | `internal/pkgmgr/catalog.json`, `internal/pkgmgr/download.go` (`verifySHA256`), `frontend/src/screens/PhpIniPanel.tsx` |
| Telas › MySQL ou MariaDB: credenciais, comando de conexão, databases e phpMyAdmin | `frontend/src/screens/Database.tsx`, `services/database_service.go` |
| Telas › mail() cai no Mailpit, dentro do app | `internal/render/phpini.go` (`SMTP`, `smtp_port`), `frontend/src/screens/Mail.tsx` |
| Telas › Trocar Apache ou nginx, MySQL ou MariaDB; se o novo não subir, o anterior continua | `internal/stack/stack.go` (`SwitchWebServer`, `SwitchDatabase`) |
| Telas › CLI para o terminal e agentes de IA, com `--json` e códigos de saída; a aba mostra quem chamou o quê | `cmd/hyphp/main.go`, `services/cli_handlers.go`, `services/cli_service.go` (`Activity`), `internal/cli/protocol.go` (`HeaderCaller`), `frontend/src/screens/Cli.tsx` |

A única exceção são as notas de versão do histórico: elas aparecem como foram publicadas, no idioma escolhido.

## Licença

As contribuições ficam sob a [licença MIT](../LICENSE) deste repositório ([CONTRIBUTING.md](../CONTRIBUTING.md)). As fontes mantêm as próprias licenças (`assets/fontes/LICENSE-*`).
