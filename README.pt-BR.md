<p align="center">
  <img src=".github/assets/banner.png" alt="HyPHP — ambiente PHP declarado por projeto. A janela do app com Apache, PHP 7.4, 8.3 e 8.4, MySQL e Mailpit no ar">
</p>

<h1 align="center">HyPHP</h1>

<p align="center">Ambiente de desenvolvimento PHP para Windows e macOS.<br>Várias versões de PHP ao mesmo tempo, cada projeto declara a sua. Sem Docker.</p>

<p align="center">
  <a href="https://github.com/arismarioneves/hyphp/releases/latest"><img src="https://img.shields.io/github/v/release/arismarioneves/hyphp?label=release&color=2563eb" alt="Última versão"></a>
  <a href="https://github.com/arismarioneves/hyphp/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/arismarioneves/hyphp/ci.yml?branch=main&label=ci" alt="CI"></a>
  <img src="https://img.shields.io/badge/platforms-Windows%20%7C%20macOS-555" alt="Windows e macOS">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-PolyForm%20Shield%201.0.0-555" alt="Licença"></a>
</p>

<p align="center">
  <a href="https://ae8.com.br/hyphp/">Site</a> ·
  <a href="https://github.com/arismarioneves/hyphp/releases/latest">Download</a> ·
  <a href="#primeiros-passos">Primeiros passos</a> ·
  <a href="#cli">CLI</a> ·
  <a href="CONTRIBUTING.md">Contribuir</a>
</p>

<p align="center"><a href="README.md">English</a> | <b>Português</b></p>

O HyPHP é um app desktop com orquestrador próprio: várias versões de PHP servindo domínios
`.test` diferentes **ao mesmo tempo**, HTTPS local, workers supervisionados e um ambiente
declarado num `hyphp.yaml` que viaja com o projeto. PHP, Apache ou nginx, MySQL ou MariaDB,
Mailpit e phpMyAdmin são baixados, conferidos e supervisionados pelo app.

## Download

| | |
|---|---|
| **Windows 10/11 x64** | [Instalador](https://github.com/arismarioneves/hyphp/releases/latest) (`hyphp-<versão>-windows-amd64-setup.exe`). Ainda não tem assinatura digital: se o SmartScreen avisar, use **Mais informações › Executar assim mesmo**. |
| **macOS 15+ em Apple Silicon** | `curl -fsSL https://github.com/arismarioneves/hyphp/releases/latest/download/install.sh \| sh` instala ou atualiza o `/Applications/HyPHP.app`. O dmg da página de Releases também serve; o macOS bloqueia a primeira abertura de um app que ele não notarizou, então libere uma vez em **Ajustes do Sistema › Privacidade e Segurança › Abrir Mesmo Assim**. Os runtimes vêm do [Homebrew](https://brew.sh). |

## Por que o HyPHP

| Ferramenta | Limitação |
|---|---|
| **XAMPP** | `mod_php`: uma versão de PHP por instalação. Sem domínios locais, sem HTTPS local, órfãos após crash. |
| **Laragon** | Arquitetura correta, mas fechado, sem fonte publicada. A versão de PHP troca por menu ou por *Profile*, que vale para a instalação inteira, e não fica declarada no projeto. |
| **WampServer** | PHP por VirtualHost via FCGI desde a 3.2.8 (changelog oficial), mas só Apache, e a configuração mora nos menus e arquivos da instalação, não no projeto. |
| **DDEV / Devilbox / Lando** | Sólidos e open source, mas exigem Docker: 2–4 GB de RAM antes do primeiro request. |

O HyPHP junta, no Windows e no macOS e sem Docker, o que os outros entregam separado:
**vários PHP atendendo ao mesmo tempo**, a versão **declarada no próprio projeto**
(`hyphp.yaml`, commitável), Apache ou nginx, e workers supervisionados.

## O que ele faz

- **Multi-versão simultânea real.** Um pool de workers por versão de PHP (`php-cgi` em modo
  FastCGI no Windows, `php-fpm` no macOS) atrás de um único Apache ou nginx. Cada projeto
  declara a sua versão.
- **Pool de workers.** No Windows o `php-cgi` atende um request por vez; sem pool, um request
  lento trava o site. Medido: 2 requests de 3 s levaram 6,2 s com 1 worker e 3 s com 2.
- **Sem processos órfãos.** No Windows, um Job Object com kill-on-close; no macOS, um grupo de
  processos por serviço. O que um crash deixou para trás é limpo na próxima abertura.
- **Estado real.** Cada serviço tem probe de readiness; a UI mostra `degraded` quando o
  processo vive mas não responde.
- **Domínios `.test` e HTTPS local.** Cada projeto responde em `<nome>.test`, com certificado
  de uma CA local (mkcert) depois que você a instala. `wildcard: true` serve `*.<nome>.test`
  pelo resolvedor de DNS do próprio app.
- **A página do localhost se explica.** `http://localhost` lista os projetos desta máquina e,
  para um host que abre a página errada, diz o motivo (subdomínio sem `wildcard: true`,
  domínio fora de `.test` apontando para esta máquina).
- **Config é saída, não entrada.** `etc/` é gerado e validado (`httpd -t` / `nginx -t`)
  antes de aplicar; config inválida nunca derruba o ambiente.
- **Apache ou nginx, MySQL ou MariaDB.** Um de cada ativo por vez, nas mesmas portas, cada
  banco com o próprio diretório de dados. A troca é imediata e volta ao anterior se o novo
  não subir.
- **Tela de banco.** Credenciais, comando de conexão, databases e phpMyAdmin a um clique.
  `database:` no `hyphp.yaml` cria o database se ele não existir.
- **E-mail fica local.** O `php.ini` aponta o `mail()` para o Mailpit; a caixa de entrada
  fica dentro do app.
- **php.ini por versão.** Diretivas como `max_input_vars` são editadas no app para cada
  versão de PHP, com o valor efetivo e de onde ele vem.
- **HTTPS de saída funciona no Windows.** Os builds de PHP para Windows vêm sem certificados
  de CA; o HyPHP mantém um pacote (o da Mozilla, mais a CA local) e aponta `curl.cainfo` e
  `openssl.cafile` para ele, então `curl`, `file_get_contents` e Composer não falham com
  "curl error 60".
- **Runtimes das fontes oficiais.** PHP 7.2 a 8.5, Apache, nginx, MySQL, MariaDB, Mailpit,
  mkcert e phpMyAdmin são baixados sob demanda e conferidos por SHA-256. O catálogo de
  pacotes é assinado e atualizado pelo site a cada 6 horas, então um link que mudou é
  corrigido sem versão nova do app.
- **CLI para o terminal e agentes de IA.** `hyphp status`, `hyphp start all`,
  `hyphp logs mysql -f`, `hyphp ini 8.3 max_input_vars 5000`… Cada comando roda no app
  aberto, com saída `--json` e códigos de saída que dizem o que houve. Veja [CLI](#cli).
- **Mora na bandeja**, pode abrir com o sistema, em português ou inglês, tema escuro ou claro
  (ou igual ao do sistema).

## Primeiros passos

1. **Instale** e abra o HyPHP (veja [Download](#download)).
2. **Runtimes:** baixe uma versão de PHP e o Apache (nginx, MySQL ou MariaDB, Mailpit e
   phpMyAdmin são opcionais). No macOS a mesma tela os instala pelo Homebrew.
3. **Projetos:** adicione a pasta onde os seus projetos ficam. Cada subpasta com um
   `hyphp.yaml`, um `public/index.php`, um `index.php` ou um `composer.json` vira um projeto
   em `<pasta>.test`.
4. **Dashboard:** os avisos dizem o que falta e oferecem a correção. As ações que precisam de
   administrador (o arquivo `hosts` e a regra de DNS do `.test`, o certificado local) só rodam
   quando você clica nelas.
5. Abra `http://acme.test`. Depois, commite um `hyphp.yaml` para o próximo clone receber o
   mesmo ambiente:

```yaml
name: acme
domain: acme.test          # padrão: <name>.test
wildcard: false            # true serve *.acme.test pelo resolvedor de DNS local
php: "8.1"                 # ausente = versão padrão global
docroot: public            # padrão: public se existir, senão a raiz
extensions: [pdo_mysql, intl, zip, gd]
database: acme_dev         # criado se não existir
processes:
  queue: php artisan queue:work --tries=3
  scheduler: php artisan schedule:work
```

Portas padrão: 80 e 443 para o web server, 3306 para o banco, 1025 (SMTP) e 8025 (web) para
o Mailpit, 8036 para o phpMyAdmin. Todas mudam em Configurações.

## CLI

O `hyphp` fala com o app aberto (no Windows por um named pipe que só o seu usuário alcança;
no macOS por um socket dentro da raiz de dados) e roda cada comando com os mesmos serviços da
janela: o que a CLI faz aparece na UI na hora, e a aba **CLI** lista as chamadas recentes e
quem as fez. Coloque-a no PATH por essa aba.

```text
hyphp status                          versão, web server, banco, serviços e avisos
hyphp services                        serviços e estados
hyphp start [serviço|all]             inicia e espera ficar pronto (sem argumento: tudo)
hyphp stop [serviço|all]              para (sem argumento: tudo)
hyphp restart <serviço>               reinicia e espera ficar pronto
hyphp logs <serviço> [-n 50] [-f]     últimas linhas do log; -f acompanha
hyphp projects                        projetos, domínio, PHP e pasta
hyphp php | php default <série> | php use <projeto> <série>
hyphp ini <série> [<diretiva> <valor> | <diretiva> --reset]
hyphp db | db create <nome> | db drop <nome> --yes | db engine <mysql|mariadb>
hyphp web <apache|nginx>              troca o web server
hyphp warnings                        avisos da stack
hyphp app                             mostra a janela (abre o app se estiver fechado)
hyphp version                         versão da CLI e do app
hyphp help [comando]                  ajuda
```

Para scripts e agentes de IA: todo comando aceita `--json` (erros também), e o código de
saída é `0` ok, `1` erro do app, `2` uso errado, `3` app fechado.

## Plataformas

| | Windows 10/11 x64 | macOS 15+ em Apple Silicon |
|---|---|---|
| Runtimes | baixados pelo app das fontes oficiais | instalados pelo app com o Homebrew (`shivammathur/php` para o PHP) |
| Workers de PHP | pools de `php-cgi` (não existe `php-fpm` no Windows) | `php-fpm` |
| Domínios `.test` | arquivo `hosts`; wildcard por uma regra NRPT | `/etc/resolver/test` |
| Raiz de dados | ao lado do executável, se a pasta for gravável; senão `%LOCALAPPDATA%\HyPHP` | `~/Library/Application Support/HyPHP` |
| Assinatura | ainda não (aviso do SmartScreen) | ad hoc, sem notarização (aviso do Gatekeeper na primeira abertura) |

`HYPHP_ROOT` sobrescreve a raiz de dados nos dois. O código de um SO só fica em arquivos
`_windows.go` e `_darwin.go`.

## Stack

| Componente | Versões no catálogo | Licença | Origem |
|---|---|---|---|
| PHP | 7.2, 7.4, 8.0, 8.1, 8.2, 8.3, 8.4, 8.5 | PHP License 3.01 | windows.php.net |
| Apache httpd | 2.4 | Apache-2.0 | apachelounge.com |
| nginx | 1.30 | BSD-2-Clause | nginx.org |
| MySQL Community | 8.0, 8.4 | GPL-2.0 | dev.mysql.com |
| MariaDB Server | 10.11, 11.4, 11.8, 12.3 | GPL-2.0 | mariadb.org |
| Mailpit | 1.31 | MIT | github.com/axllent/mailpit |
| mkcert | 1.4 | BSD-3-Clause | github.com/FiloSottile/mkcert |
| phpMyAdmin | 5.2 | GPL-2.0 | phpmyadmin.net |
| Certificados de CA | pacote da Mozilla | MPL-2.0 | curl.se |

O HyPHP não redistribui esses binários: baixa sob demanda, confere o SHA-256 e extrai em
`bin/` (no macOS, o Homebrew os instala). Um build compatível colocado à mão em `bin/`
também é reconhecido. O catálogo com os links e os hashes fica em
[`internal/pkgmgr/catalog.json`](internal/pkgmgr/catalog.json).

Feito em Go, [Wails v3](https://wails.io), React e TypeScript.

## Instalação em detalhe

### Windows

O instalador publicado instala em Arquivos de Programas e pede UAC uma vez. Modo silencioso:

```powershell
hyphp-<versão>-windows-amd64-setup.exe /S /D=C:\caminho    # pasta à sua escolha
```

O app em si nunca precisa de privilégio para funcionar: a raiz de dados (`bin/`, `etc/`,
`var/`, `log/`) fica ao lado do executável se a pasta for gravável; senão, em
`%LOCALAPPDATA%\HyPHP`. Um instalador por usuário (sem UAC) sai do código fonte
(`INSTALL_SCOPE=user`, veja [Desenvolvimento](#desenvolvimento)).

O instalador traz `hyphp.exe` e `hyphp-helper.exe`. O helper faz as ações que precisam de
administrador (gravar o `hosts`, instalar o certificado raiz local, a regra de DNS do `.test`,
instalar uma atualização); cada uma pede UAC só quando você clica nela.

Desinstalar remove a pasta da instalação, mas **não** desfaz o que você aplicou ao sistema:
tire o bloco do `hosts`, a regra de DNS do `.test` e a entrada de início automático em
**Configurações** (o card **Permissões** e o interruptor **Iniciar com o Windows**) antes de
desinstalar.

### macOS

```bash
curl -fsSL https://github.com/arismarioneves/hyphp/releases/latest/download/install.sh | sh
curl -fsSL https://github.com/arismarioneves/hyphp/releases/latest/download/uninstall.sh | sh
```

O `install.sh` confere o SHA-256 do dmg contra o manifesto da release, fecha o HyPHP se
estiver aberto e só pede a senha quando `/Applications` não é gravável. O `uninstall.sh`
fecha o app, remove a regra de DNS do `.test`, o `/etc/paths.d/hyphp` e a confiança na CA do
mkcert (pedindo a senha de administrador), depois a entrada de início automático, a pasta
`cli` e o app. Os dados (bancos, configurações, logs) ficam, a menos que você confirme no
Terminal ou rode `uninstall.sh | sh -s -- --apagar-dados`. O Homebrew, as fórmulas dele e os
arquivos da CA do mkcert não são tocados.

### Atualizações

O app procura a versão mais nova um minuto depois de abrir e a cada 6 horas (dá para
desligar em **Configurações › Atualizações**) e a baixa em segundo plano. O manifesto da
release é assinado com ed25519 (chave pública em `internal/update/key.go`) e traz o SHA-256 do
instalador; o app recusa manifesto ou instalador que não confira. A instalação só acontece
quando você clica em **Atualizar e reiniciar**: os serviços param, o sistema pede permissão
uma vez (Windows) e o app volta na versão nova. No macOS o app troca o próprio bundle pelo do
dmg novo, sem senha quando a sua conta grava em `/Applications`; senão, mostra o comando do
Terminal acima.

## Desenvolvimento

Precisa de Go 1.26+, Node 22+ e da CLI do Wails v3:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.23
wails3 doctor     # confere o ambiente
wails3 dev        # desenvolvimento com hot reload
wails3 build      # binário em bin/
```

Empacotar:

```powershell
# Windows: NSIS no PATH (winget install NSIS.NSIS instala em C:\Program Files (x86)\NSIS)
$env:PATH = 'C:\Program Files (x86)\NSIS;' + $env:PATH
wails3 task windows:package                      # bin/hyphp-amd64-installer.exe
wails3 task windows:package INSTALL_SCOPE=user   # escopo de usuário, sem UAC
```

```bash
# macOS
wails3 task darwin:package       # bin/HyPHP.app (assinado ad hoc, CLI e helper em Contents/Helpers)
wails3 task darwin:package:dmg   # bin/HyPHP.dmg
```

As regras do projeto (comentários, textos, testes, commits, checagens antes de um pull
request) estão em [AGENTS.md](AGENTS.md). O site fica em [`web/`](web/), com regras próprias
em [web/README.pt-BR.md](web/README.pt-BR.md).

## Apoie o projeto

O HyPHP é gratuito. Se ele economiza o seu tempo, você pode apoiar o desenvolvimento:

[![Buy Me a Coffee](https://img.shields.io/badge/Buy%20Me%20a%20Coffee-arismarioneves-FFDD00?logo=buymeacoffee&logoColor=000000)](https://buymeacoffee.com/arismarioneves)

## Licença

O código é aberto para leitura, no modelo *open core*:

- **Tudo fora de `enterprise/`** usa a [PolyForm Shield 1.0.0](LICENSE). Qualquer
  pessoa ou empresa pode usar o HyPHP de graça, inclusive no trabalho, e estudar,
  modificar e compartilhar o código. O que ela não permite é oferecer um produto
  que concorra com o HyPHP ou com os recursos pagos dele, seja pago ou gratuito,
  o que inclui vender o próprio HyPHP.
- **`enterprise/`** vai guardar os recursos pagos, com [licença própria](enterprise/LICENSE)
  e uso por assinatura. Hoje ela só tem a licença: ainda não existe recurso pago.

Ao enviar um pull request, você concorda com o [CLA](CLA.md); ver
[CONTRIBUTING.md](CONTRIBUTING.md) (em inglês). Os componentes que o HyPHP baixa (tabela em
[Stack](#stack)) seguem cada um a própria licença.
