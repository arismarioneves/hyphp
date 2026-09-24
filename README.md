# HyPHP

Ambiente de desenvolvimento PHP com orquestrador próprio: várias versões de PHP servindo
domínios diferentes **ao mesmo tempo**, HTTPS local, workers supervisionados e ambiente
reproduzível por projeto.

> **Status:** em desenvolvimento. O design está fechado e validado por experimento; a
> implementação está planejada em `docs/superpowers/plans/`. Ainda não há binário utilizável.

## Por que existe

| Ferramenta | Limitação |
|---|---|
| **XAMPP** | `mod_php`: uma versão de PHP por instalação. Sem domínios locais, sem HTTPS local, órfãos após crash. |
| **Laragon** | Arquitetura correta, mas fechado, sem fonte publicada e com licença exigida a partir da 7.x. Versão de PHP é global, não por projeto; workers não são supervisionados. |
| **WampServer** | Multi-versão via `mod_php`: só uma versão atende por vez, e a troca reinicia tudo. |
| **DDEV / Devilbox / Lando** | Sólidos e open source, mas exigem Docker — 2–4 GB de RAM antes do primeiro request. |

O HyPHP resolve o caso que nenhum deles resolve bem no Windows: **PHP 7.2 e 8.3 atendendo
simultaneamente**, cada projeto declarando sua versão, sem Docker e sem `mod_php`.

## O que faz diferente

- **Multi-versão simultânea real** — `php-cgi` em modo FastCGI externo, um pool por versão,
  um único Apache (ou nginx) na frente. Versão declarada por projeto.
- **Pool de workers** — `php-cgi` no Windows atende um request por vez; sem pool, um request
  lento trava o site. Medido: 2 requests de 3 s levaram 6,2 s em 1 worker e 3 s em 2.
- **Zero processos órfãos** — garantido pelo kernel, via Job Object com kill-on-close.
- **Estado real** — cada serviço tem probe de readiness; a UI mostra `degraded` quando o
  processo vive mas não responde.
- **Ambiente reproduzível** — `hyphp.yaml` commitado no repositório do projeto.
- **Config é saída, não entrada** — `etc/` é gerado e validado (`httpd -t` / `nginx -t`)
  antes de aplicar; config inválida nunca derruba o ambiente.

## Plataformas

A v1 tem como alvo **Windows 10/11 x64** — deliberadamente o caso mais difícil, já que
`php-fpm` não existe nessa plataforma. A arquitetura isola o que é específico de sistema
operacional em arquivos com build tag; portar para macOS e Linux é mais simples, porque lá
`php-fpm` existe e elimina a peça mais complexa do design. Ver §16 da especificação.

## `hyphp.yaml`

```yaml
name: acme
domain: acme.test          # default: <name>.test
wildcard: false            # true habilita *.acme.test via resolvedor DNS local
php: "8.1"                 # ausente = versão padrão global
docroot: public            # default: public se existir, senão a raiz
extensions: [pdo_mysql, intl, zip, gd]
database: acme_dev         # criado se ausente
processes:
  queue: php artisan queue:work --tries=3
  scheduler: php artisan schedule:work
```

## Documentação

| Documento | Conteúdo |
|---|---|
| `docs/superpowers/specs/2026-09-18-hyphp-design.md` | Especificação: decisões, evidências dos experimentos, riscos abertos |
| `docs/superpowers/plans/2026-09-18-hyphp-00-index.md` | Contratos compartilhados (tipos, assinaturas, eventos) e ordem de execução |
| `docs/superpowers/plans/2026-09-18-hyphp-0*.md` | Planos de implementação por subsistema |
| `docs/superpowers/reference/wails3-api.md` | Referência da API do Wails v3 usada pelo projeto |

## Stack

Go 1.27 · Wails v3 (WebView2) · React 18 + TypeScript · Tailwind v4 · Phosphor Icons

Componentes orquestrados, baixados das fontes oficiais, cada um sob a própria licença:

| Componente | Licença | Origem |
|---|---|---|
| PHP | PHP License 3.01 | windows.php.net |
| Apache httpd | Apache-2.0 | apachelounge.com |
| nginx | BSD-2-Clause | nginx.org |
| MySQL Community | GPL-2.0 | dev.mysql.com |
| Mailpit | MIT | github.com/axllent/mailpit |
| mkcert | BSD-3-Clause | github.com/FiloSottile/mkcert |

O HyPHP não redistribui esses binários: baixa sob demanda, verifica o SHA-256 e extrai em
`bin/`. Qualquer build compatível colocada manualmente em `bin/` também é reconhecida.

## Desenvolvimento

Requer Go 1.26+, Node/npm e a CLI do Wails v3:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.23
wails3 doctor     # verifica o ambiente
wails3 dev        # desenvolvimento com hot reload
wails3 build      # binário em bin/
```

Gerar o instalador exige o [NSIS](https://nsis.sourceforge.io/) no `PATH`
(`winget install NSIS.NSIS` instala em `C:\Program Files (x86)\NSIS`, que o
instalador não acrescenta ao `PATH` automaticamente):

```powershell
$env:PATH = 'C:\Program Files (x86)\NSIS;' + $env:PATH
wails3 task windows:package   # bin/hyphp-amd64-installer.exe
```

O instalador leva `hyphp.exe` e `hyphp-helper.exe`. O helper é o binário com
manifesto `requireAdministrator` que executa as duas únicas ações elevadas
(escrever no `hosts`, instalar o certificado raiz local); sem ele ao lado do
executável principal, essas ações falham.

### Instalação

O instalador aceita os dois escopos do NSIS:

```powershell
wails3 task windows:package                      # máquina (Program Files, pede UAC)
wails3 task windows:package INSTALL_SCOPE=user   # usuário (sem UAC)
hyphp-amd64-installer.exe /S /D=C:\caminho       # silencioso, diretório à escolha
```

A raiz de dados (`bin/`, `etc/`, `var/`, `log/`) fica **ao lado do executável se
esse diretório for gravável**, senão em `%LOCALAPPDATA%\HyPHP`. É o que permite
instalar em Program Files sem que o app precise de privilégio para funcionar.
`HYPHP_ROOT` sobrepõe a regra.

A desinstalação remove o diretório de instalação, mas **não** desfaz o que o
usuário aplicou no sistema: o bloco do `hosts`, a regra de DNS `.test` e a
entrada de autostart saem pela própria interface (card **Permissões** em
Configurações e o toggle de início automático), antes de desinstalar.

## Relação com o Laragon

O HyPHP é implementação limpa. O Laragon não tem licença open-source (a API do GitHub
reporta `"license": null`) e não publica código-fonte — o repositório contém apenas o
binário compilado. Reaproveitamos **ideias e comportamento** (domínios `.test`, orquestração
própria, layout portátil), que não são protegidos por copyright, e nenhum arquivo de
configuração, template ou artefato derivado do executável dele.
