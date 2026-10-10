#!/bin/bash
# Teste de ponta a ponta do HyPHP no macOS, rodado pelo job macos-e2e do ci.yml
# num runner macos-15 (Apple Silicon, sessão gráfica, Homebrew instalado).
#
# Repete sem ninguém na tela o roteiro manual da M1, da M2 e da M3:
#   - runtimes instalados pelo Homebrew fora do app;
#   - um projeto em ~/Code;
#   - o app instalado em /Applications pelo install.sh, com o dmg do build
#     servido por um servidor local no lugar do GitHub;
#   - checagens pelo curl e pela CLI do bundle;
#   - a regra de DNS, a confiança na CA e o PATH registrados pelo helper, com o
#     sudo do runner no lugar da janela de senha;
#   - o projeto em HTTPS, confiado pelo sistema;
#   - o app morto por kill -9 e reaberto, que é o caminho da limpeza de órfãos;
#   - um update aplicado pelo atualizador do app, com o dmg do build;
#   - o app fechado normalmente e as mudanças removidas do sistema;
#   - a reinstalação pelo install.sh com o app aberto e a desinstalação pelo
#     uninstall.sh, mantendo e depois apagando os dados.
#
# Escrito para o bash 3.2 do macOS: sem arrays vazios sob `set -u`.
#
# Uso: e2e-macos.sh <caminho do HyPHP.dmg>
set -euo pipefail

if [ -z "${CI:-}" ]; then
	# Apaga e semeia a pasta de dados do HyPHP: num Mac de verdade isso
	# destruiria o estado do usuário.
	echo "e2e-macos.sh roda só no CI (variável CI ausente)" >&2
	exit 2
fi

DMG=$(cd "$(dirname "${1:?uso: e2e-macos.sh <HyPHP.dmg>}")" && pwd)/$(basename "$1")
REPO=$(cd "$(dirname "$0")/../.." && pwd)
APP=/Applications/HyPHP.app
CLI="$APP/Contents/Helpers/hyphp"
MAIN="$APP/Contents/MacOS/hyphp"
ROOT="$HOME/Library/Application Support/HyPHP"
CODE="$HOME/Code"
SITE="http://127.0.0.1"
HOST="Host: teste.test"
ASSUNTO="hyphp-e2e-$RANDOM$RANDOM"
CORPO=$(mktemp)
# O install.sh baixa daqui, como baixaria de releases/latest/download/.
SERVIDOR=http://127.0.0.1:18080
SERVIDO=$(mktemp -d)
SERVIDOR_PID=

passo() { printf '\n==> %s\n' "$*"; }
falha() {
	printf '::error::%s\n' "$*"
	exit 1
}

# ao_sair derruba o servidor do install.sh e, quando o script falha, despeja o
# estado do app e os logs: o ci.yml não guarda artefatos, então o log do job é
# tudo o que sobra para investigar.
ao_sair() {
	local status=$?
	[ -z "$SERVIDOR_PID" ] || kill "$SERVIDOR_PID" 2>/dev/null || true
	[ "$status" -eq 0 ] && return
	echo "::group::diagnóstico"
	"$CLI" status --json || true
	"$CLI" services --json || true
	"$CLI" warnings --json || true
	lsof -nP -iTCP -sTCP:LISTEN || true
	pgrep -fl 'hyphp|httpd|php-fpm|mysqld|mailpit' || true
	ls -la "$ROOT/var/run/procs" /Applications || true
	for f in "$ROOT"/log/*.log "$ROOT/var/update/aplicar.log"; do
		[ -f "$f" ] || continue
		echo "--- $f"
		tail -n 80 "$f"
	done
	echo "::endgroup::"
}
trap ao_sair EXIT

# get faz o GET e deixa a resposta em $CORPO. Fora do 200, mostra o fim do
# corpo (onde o PHP escreve o erro) antes de falhar.
get() {
	local codigo
	codigo=$(curl -sS -o "$CORPO" -w '%{http_code}' "$@") || falha "curl falhou: $*"
	if [ "$codigo" != 200 ]; then
		tail -c 2000 "$CORPO"
		echo
		falha "HTTP $codigo em: $*"
	fi
}

# esperar_pronto espera a CLI responder com todos os serviços prontos e os
# quatro grupos do roteiro presentes. Antes disso o socket pode nem existir.
esperar_pronto() {
	local limite=$((SECONDS + $1)) s
	while [ "$SECONDS" -lt "$limite" ]; do
		if s=$("$CLI" services --json 2>/dev/null) &&
			jq -e 'length > 0 and all(.[]; .state == "ready")
				and ([.[].group] | contains(["web", "php", "db", "mail"]))' <<<"$s" >/dev/null; then
			return 0
		fi
		sleep 3
	done
	falha "serviços não ficaram prontos em $1 s"
}

# esperar_saida espera o processo principal do app terminar.
esperar_saida() {
	local limite=$((SECONDS + $1))
	while pgrep -f "$MAIN" >/dev/null; do
		[ "$SECONDS" -lt "$limite" ] || falha "o app não terminou em $1 s"
		sleep 1
	done
}

# pids lista "pid id" de cada serviço com processo.
pids() {
	"$CLI" services --json | jq -r '.[] | select(.pid > 0) | "\(.pid) \(.id)"'
}

# conferir_avisos mostra os avisos e exige exatamente os códigos dados, em
# qualquer ordem. Sem argumento, exige nenhum: com a regra de DNS e a CA
# registradas, o Mac não deixa pendência.
conferir_avisos() {
	local esperados
	esperados=$(printf '%s\n' "$@" | jq -R . | jq -sc 'map(select(. != "")) | sort')
	"$CLI" warnings
	"$CLI" warnings --json | jq -e --argjson e "$esperados" '[.[].code] | unique == ($e | unique)' >/dev/null ||
		falha "avisos diferentes de $esperados"
}

# helper roda o hyphp-helper do bundle com sudo. O sudo do runner não pede
# senha e faz o papel da janela do osascript: o helper roda como root, como o
# app o chamaria.
helper() {
	local out
	out=$(sudo "$APP/Contents/Helpers/hyphp-helper" "$@") || true
	echo "$out"
	grep -q '"ok":true' <<<"$out" || falha "hyphp-helper $1"
}

# fechar_app fecha pelo mesmo caminho do menu Sair e exige que nenhum
# serviço nem registro de processo sobre.
fechar_app() {
	local depois
	depois=$(pids)
	# SIGTERM cai no mesmo app.Quit do menu Sair (handler de sinal do Wails). O
	# osascript "quit" precisaria da permissão de Automação, que o runner não dá.
	pkill -TERM -f "$MAIN"
	esperar_saida 90
	while read -r pid id; do
		if ps -p "$pid" >/dev/null; then
			falha "o serviço $id (pid $pid) ficou vivo depois de fechar o app"
		fi
	done <<<"$depois"
	if compgen -G "$ROOT/var/run/procs/*.json" >/dev/null; then
		falha "registros de processo sobraram depois de fechar o app: $(ls "$ROOT/var/run/procs")"
	fi
}

# tentar repete um comando da CLI até ele ser aceito. Logo depois de um brew
# install, o app ainda pode não ter relido o opt/ do Homebrew: o watcher reage
# em segundo plano.
tentar() {
	local limite=$((SECONDS + $1))
	shift
	until "$CLI" "$@"; do
		[ "$SECONDS" -lt "$limite" ] || falha "hyphp $* não foi aceito a tempo"
		sleep 3
	done
}

# instalar roda o `brew install` como o app (internal/brew/exec_darwin.go): uma
# fórmula por vez, o PHP pelo nome completo do tap, sem tap/trust, e a saída 1
# com o keg no opt/ conta como sucesso, porque só o link em bin/ falhou (por
# exemplo, mariadb@11.8 ao lado do mysql@8.4).
instalar() {
	brew install "$1" || [ -d "$(brew --prefix)/opt/${1##*/}" ] || falha "brew install $1"
}

# sem_sobras_da_troca exige que a troca do bundle (install.sh ou atualizador)
# não tenha deixado o .novo nem o .antigo em /Applications.
sem_sobras_da_troca() {
	local f
	for f in /Applications/.HyPHP.app.novo /Applications/.HyPHP.app.antigo; do
		[ ! -e "$f" ] || falha "a troca do app deixou $f"
	done
}

# instalar_pelo_script roda o install.sh como o usuário rodaria pelo curl e
# exige o app assinado no lugar. O script fecha o app aberto e o reabre.
instalar_pelo_script() {
	HYPHP_URL_BASE="$SERVIDOR" sh "$REPO/build/darwin/install.sh" || falha "o install.sh falhou"
	sem_sobras_da_troca
	codesign --verify --deep --strict "$APP" || falha "o app instalado não passa no codesign"
}

# ca_no_keychain diz se a CA do mkcert está no keychain do sistema, pelo
# SHA-1, como o CAInstalled do app confere. A listagem vem antes do grep: no
# pipe, o grep -q sairia no primeiro acerto, o security levaria SIGPIPE (141)
# e, com pipefail, o resultado seria falso justo quando a CA ficou.
ca_no_keychain() {
	local sha1 listagem
	sha1=$(openssl x509 -in "$CAROOT_DIR/rootCA.pem" -noout -fingerprint -sha1 | cut -d= -f2 | tr -d :)
	listagem=$(security find-certificate -a -Z /Library/Keychains/System.keychain)
	grep -q "SHA-1 hash: $sha1" <<<"$listagem"
}

passo "runtimes pelo Homebrew"
export HOMEBREW_NO_ENV_HINTS=1 HOMEBREW_NO_COLOR=1 HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1
for f in shivammathur/php/php@8.3 httpd mysql@8.4 mailpit mkcert; do
	instalar "$f"
done
ls -ld "$(brew --prefix)"/opt/{php*,httpd,mysql*,mariadb*,mailpit,nginx} 2>/dev/null || true

passo "phpMyAdmin do catálogo"
# O app baixa o phpMyAdmin pela tela Runtimes, que o teste não alcança. Aqui vai
# o mesmo zip do catálogo, conferido pelo mesmo sha256, para a pasta onde o
# pkgmgr o colocaria (bin/phpmyadmin/<raiz do zip>).
catalogo="$REPO/internal/pkgmgr/catalog.json"
pma_url=$(jq -r '.packages[] | select(.kind == "phpmyadmin") | .url' "$catalogo")
pma_sha=$(jq -r '.packages[] | select(.kind == "phpmyadmin") | .sha256' "$catalogo")
curl -fsSL -o "$CORPO.zip" "$pma_url"
echo "$pma_sha  $CORPO.zip" | shasum -a 256 -c -
mkdir -p "$ROOT/bin/phpmyadmin"
ditto -x -k "$CORPO.zip" "$ROOT/bin/phpmyadmin"

passo "estado inicial e projeto"
# Sem tela para clicar em "Adicionar diretório", a raiz entra direto no
# state.json, onde a tela a grava. O defaultPhp fixo impede que outro PHP da
# imagem do runner vire o PHP do projeto.
mkdir -p "$ROOT/var" "$CODE/teste"
cat >"$ROOT/var/state.json" <<EOF
{"schemaVersion": 1, "defaultPhp": "8.3", "roots": ["$CODE"]}
EOF
cat >"$CODE/teste/index.php" <<'EOF'
<?php
header('Content-Type: text/plain');
echo 'php=', PHP_VERSION, "\n";
EOF
# "localhost" faz o mysqli usar o socket padrão, o que confere o
# mysqli.default_socket que o HyPHP grava no php.ini do Mac.
cat >"$CODE/teste/db.php" <<'EOF'
<?php
header('Content-Type: text/plain');
$db = new mysqli('localhost', 'root', '', 'hyphp_e2e');
echo 'mysql=', $db->server_info, "\n";
EOF
cat >"$CODE/teste/mail.php" <<'EOF'
<?php
header('Content-Type: text/plain');
echo mail('dev@exemplo.test', $_GET['assunto'], "corpo\n") ? "mail=ok\n" : "mail=falhou\n";
EOF

passo "portas antes de abrir o app"
lsof -nP -iTCP -sTCP:LISTEN || true

passo "servir o dmg e o latest.json"
# O latest.json no formato do hyphp-release, com um instalador do Windows de
# mentira ao lado: é o arquivo inteiro que o plutil do install.sh tem de ler.
VERSAO=$(plutil -extract CFBundleShortVersionString raw -o - "$REPO/build/darwin/Info.plist")
NOME="hyphp-$VERSAO-darwin-arm64.dmg"
SHA=$(shasum -a 256 "$DMG" | cut -d ' ' -f 1)
TAM=$(stat -f %z "$DMG")
cp "$DMG" "$SERVIDO/$NOME"
jq -n --arg v "$VERSAO" --arg n "$NOME" --arg s "$SHA" --argjson t "$TAM" '{
	schema: 1, version: $v, date: "2026-10-11",
	notes: ["nota do e2e"], notes_en: ["e2e note"],
	windows_amd64: {path: ("hyphp-" + $v + "-windows-amd64-setup.exe"), size: 1, sha256: ("0" * 64)},
	darwin_arm64: {path: $n, size: $t, sha256: $s}
}' >"$SERVIDO/latest.json"
python3 -m http.server 18080 --bind 127.0.0.1 --directory "$SERVIDO" >/dev/null 2>&1 &
SERVIDOR_PID=$!
for _ in $(seq 20); do
	curl -fsS -o /dev/null "$SERVIDOR/latest.json" && break
	sleep 0.5
done
curl -fsS "$SERVIDOR/latest.json" || falha "o servidor local não respondeu"

passo "instalar pelo install.sh"
[ ! -e "$APP" ] || falha "o runner já tem um $APP"
instalar_pelo_script
esperar_pronto 300
"$CLI" status
"$CLI" services
conferir_avisos ca-pending wildcard-pending
"$CLI" status --json | jq -e '.failed == 0 and .ready == .total' >/dev/null ||
	falha "status com serviço em falha"

passo "projeto pelo Apache na porta 80"
get -H "$HOST" "$SITE/"
cat "$CORPO"
grep -q '^php=8\.3\.' "$CORPO" || falha "teste.test não respondeu com o PHP 8.3"
# A página de host sem projeto lista os projetos para esta máquina. A regra de
# DNS ainda não existe: teste.test só abre com o cabeçalho Host.
get "$SITE/dados/projetos.json"
cat "$CORPO"
jq -e '.projetos[] | select(.domain == "teste.test") | .abre == false and .motivo == "dns"' "$CORPO" >/dev/null ||
	falha "projetos.json sem teste.test pendente da regra de DNS"
get -H "Host: nada.test" "$SITE/"
grep -q 'id="textos"' "$CORPO" || falha "host sem projeto não serviu a página do HyPHP"

passo "banco pela CLI e pelo PHP"
"$CLI" db create hyphp_e2e
get -H "$HOST" "$SITE/db.php"
cat "$CORPO"
grep -q '^mysql=8\.4\.' "$CORPO" || falha "o PHP não conectou no MySQL 8.4 pelo socket"

passo "mail() até o Mailpit"
get -H "$HOST" "$SITE/mail.php?assunto=$ASSUNTO"
cat "$CORPO"
grep -q '^mail=ok' "$CORPO" || falha "mail() devolveu false"
# O sendmail do Mailpit entrega por SMTP antes de sair, mas a API pode levar
# um instante para listar a mensagem.
achou=
for _ in $(seq 20); do
	if curl -fsS http://127.0.0.1:8025/api/v1/messages |
		jq -e --arg s "$ASSUNTO" 'any(.messages[]; .Subject == $s)' >/dev/null; then
		achou=1
		break
	fi
	sleep 1
done
[ -n "$achou" ] || falha "a mensagem $ASSUNTO não chegou ao Mailpit"

passo "phpMyAdmin"
# A página de bancos é montada no servidor: o banco criado acima só aparece se
# o phpMyAdmin conectou no MySQL.
get "http://127.0.0.1:8036/index.php?route=/server/databases"
grep -q 'hyphp_e2e' "$CORPO" || falha "o phpMyAdmin não listou o banco hyphp_e2e"

passo "regra de DNS, certificado e PATH pelo helper"
helper resolver-write --port 15353
cat /etc/resolver/test
# O passo de usuário do InstallCA: a CA nasce com dono do usuário, sem tocar
# no keychain; só a confiança passa pelo helper.
CAROOT_DIR=$(mkcert -CAROOT)
CAROOT="$CAROOT_DIR" TRUST_STORES=nss mkcert -install
helper ca-trust --cert "$CAROOT_DIR/rootCA.pem"
mkdir -p "$ROOT/cli"
helper paths-write --dir "$ROOT/cli"
# O resolvedor já está no ar: o nome resolve de verdade, sem o cabeçalho Host.
get "http://teste.test/"
cat "$CORPO"
grep -q '^php=8\.3\.' "$CORPO" || falha "teste.test não resolveu pela regra de DNS"

passo "kill -9 no app"
antes=$(pids)
echo "$antes"
ls -l "$ROOT/var/run/procs"
pkill -9 -f "$MAIN"
esperar_saida 30
# Os serviços têm grupo próprio e seguem vivos com o app morto, a menos que
# morram sozinhos (por exemplo, SIGPIPE ao escrever no stderr que o app lia).
# Cada sobra é guardada com o executável, porque depois de uma morte o mesmo
# pid pode voltar para outro processo.
sobras=
while read -r pid id; do
	exe=$(ps -o comm= -p "$pid") || continue
	echo "órfão vivo: $id, pid $pid, ppid $(ps -o ppid= -p "$pid"), $exe"
	sobras="$sobras$pid $exe"$'\n'
done <<<"$antes"
[ -n "$sobras" ] ||
	echo "::warning::nenhum serviço sobreviveu ao kill -9; a limpeza de órfãos não foi exercitada"

passo "reabrir o app"
open "$APP"
esperar_pronto 300
"$CLI" services
while read -r pid exe; do
	[ -n "$pid" ] || continue
	if [ "$(ps -o comm= -p "$pid" 2>/dev/null)" = "$exe" ]; then
		falha "o órfão pid $pid ($exe) continua vivo depois da reabertura"
	fi
done <<<"$sobras"
if [ -n "$sobras" ]; then
	grep -F 'encerrando serviço órfão de uma execução anterior' "$ROOT/log/hyphp.log" ||
		falha "o log não registra a limpeza dos órfãos"
fi
get -H "$HOST" "$SITE/"
grep -q '^php=8\.3\.' "$CORPO" || falha "teste.test não respondeu depois da reabertura"
# A reabertura roda um Reconcile completo: agora sem pendência nenhuma.
conferir_avisos
# Com a regra de DNS e a CA, a reabertura marca teste.test como aberto e em HTTPS.
get "$SITE/dados/projetos.json"
jq -e '.projetos[] | select(.domain == "teste.test") | .abre and .https' "$CORPO" >/dev/null ||
	falha "projetos.json não marcou teste.test aberto e em HTTPS"
get --cacert "$CAROOT_DIR/rootCA.pem" "https://teste.test/"
grep -q '^php=8\.3\.' "$CORPO" || falha "teste.test não respondeu em HTTPS"
# O sistema confia no certificado do site: a cadeia fecha no keychain, não no --cacert.
openssl s_client -connect 127.0.0.1:443 -servername teste.test </dev/null 2>/dev/null | openssl x509 >"$CORPO.leaf"
security verify-cert -c "$CORPO.leaf" -p ssl -s teste.test || falha "o sistema não confia no certificado de teste.test"
achado=$(zsh -lc 'command -v hyphp' | tail -n 1) || true
[ "$achado" = "$ROOT/cli/hyphp" ] || falha "o Terminal acha $achado, não o hyphp da pasta cli"
zsh -lc 'hyphp version'

passo "nginx e MariaDB instalados com o app aberto"
# Como depois de uma instalação pela tela Runtimes: o app tem de enxergar as
# fórmulas novas sozinho, pelo watcher do opt/, sem reabrir.
for f in nginx mariadb@11.8; do
	instalar "$f"
done
tentar 90 web nginx
tentar 90 db engine mariadb
esperar_pronto 180
"$CLI" services
"$CLI" services --json | jq -e 'any(.[]; .id == "web:nginx" and .state == "ready")' >/dev/null ||
	falha "o nginx não está no ar depois da troca"
"$CLI" db --json | jq -e '.engine == "mariadb"' >/dev/null || falha "o banco ativo não é o MariaDB"
conferir_avisos

passo "projeto e banco pelo nginx e MariaDB"
get -H "$HOST" "$SITE/"
cat "$CORPO"
grep -q '^php=8\.3\.' "$CORPO" || falha "teste.test não respondeu pelo nginx"
get --cacert "$CAROOT_DIR/rootCA.pem" "https://teste.test/"
grep -q '^php=8\.3\.' "$CORPO" || falha "teste.test não respondeu em HTTPS pelo nginx"
get "$SITE/dados/projetos.json"
jq -e 'any(.projetos[]; .domain == "teste.test")' "$CORPO" >/dev/null || falha "o nginx não serviu o projetos.json"
# Cada motor tem o próprio datadir: o banco do MySQL não existe no MariaDB.
"$CLI" db create hyphp_e2e
get -H "$HOST" "$SITE/db.php"
cat "$CORPO"
grep -q '^mysql=.*MariaDB' "$CORPO" || falha "o PHP não conectou no MariaDB pelo socket"
get "http://127.0.0.1:8036/index.php?route=/server/databases"
grep -q 'hyphp_e2e' "$CORPO" || falha "o phpMyAdmin pelo nginx não listou o banco do MariaDB"

passo "update aplicado pelo atualizador"
# O pedido que o Launch do app monta (internal/update/apply_darwin.go): a
# cópia do executável em var/update, o dmg conferido por tamanho e sha256 e a
# pasta do executável no bundle. O app sai pelo SIGTERM, que cai no mesmo
# app.Quit que o Launch chama, e o atualizador troca o bundle e o reabre.
UPD="$ROOT/var/update"
mkdir -p "$UPD"
rm -f "$UPD/hyphp-updater"
cp "$MAIN" "$UPD/hyphp-updater"
app_pid=$(pgrep -f "$MAIN")
"$UPD/hyphp-updater" --aplicar-update --pid "$app_pid" --instalador "$DMG" \
	--sha256 "$SHA" --tamanho "$TAM" --dir "$APP/Contents/MacOS" --exe hyphp \
	--resultado "$UPD/resultado.json" --de 0.0.0 --para "$VERSAO" &
atualizador=$!
pkill -TERM -f "$MAIN"
if ! wait "$atualizador"; then
	cat "$UPD/aplicar.log" || true
	falha "o atualizador terminou com erro"
fi
cat "$UPD/aplicar.log"
grep -q 'resultado: ok=true' "$UPD/aplicar.log" || falha "o atualizador não registrou sucesso"
sem_sobras_da_troca
codesign --verify --deep --strict "$APP" || falha "o app trocado não passa no codesign"
esperar_pronto 300
# O app reaberto lê o resultado.json no Start, que pode vir depois dos
# serviços prontos.
leu=
for _ in $(seq 30); do
	if grep -F 'resultado do último update' "$ROOT/log/hyphp.log" | grep -q 'ok=true'; then
		leu=1
		break
	fi
	sleep 1
done
[ -n "$leu" ] || falha "o app reaberto não leu o resultado do update"

passo "fechar o app"
fechar_app

passo "remover do sistema pelo helper"
helper uninstall --cert "$CAROOT_DIR/rootCA.pem"
[ ! -e /etc/resolver/test ] || falha "a regra de DNS ficou"
[ ! -e /etc/paths.d/hyphp ] || falha "o /etc/paths.d/hyphp ficou"
# A CA tem de sumir do keychain do sistema, conferida pelo SHA-1 como o
# CAInstalled do app faz: o verify-cert na raiz seguiu dando 0 depois da
# remoção no spike, então só a folha serve para conferir a confiança.
if ca_no_keychain; then
	falha "a CA continua no keychain do sistema"
fi
if security verify-cert -c "$CORPO.leaf" -p ssl -s teste.test; then
	falha "o sistema ainda confia no certificado do site"
fi

passo "reabrir sem as mudanças no sistema"
open "$APP"
esperar_pronto 300
conferir_avisos ca-pending wildcard-pending

passo "reinstalar pelo install.sh com o app aberto"
# O caminho de quem atualiza pelo Terminal: o script fecha o app, troca o
# bundle e o reabre.
instalar_pelo_script
esperar_pronto 300

passo "desinstalar pelo uninstall.sh, mantendo os dados"
# O que o app grava no sistema volta, para o uninstall.sh ter o que desfazer;
# o plist é o que internal/autostart/autostart_darwin.go grava.
helper resolver-write --port 15353
helper ca-trust --cert "$CAROOT_DIR/rootCA.pem"
helper paths-write --dir "$ROOT/cli"
AGENTE="$HOME/Library/LaunchAgents/com.hyphp.app.plist"
mkdir -p "$(dirname "$AGENTE")"
cat >"$AGENTE" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.hyphp.app</string>
	<key>ProgramArguments</key>
	<array>
		<string>$MAIN</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
EOF
# Sem terminal o uninstall.sh não pergunta e mantém os dados. Se o runner
# tivesse um, o read do script esperaria uma resposta até o timeout do job.
if (exec </dev/tty) 2>/dev/null; then
	falha "o runner tem /dev/tty; o caminho sem terminal não seria exercitado"
fi
sh "$REPO/build/darwin/uninstall.sh" </dev/null || falha "o uninstall.sh falhou"
! pgrep -f "$MAIN" >/dev/null || falha "o app continuou aberto depois do uninstall.sh"
for f in "$APP" "$AGENTE" "$ROOT/cli" /etc/resolver/test /etc/paths.d/hyphp; do
	[ ! -e "$f" ] || falha "$f ficou depois do uninstall.sh"
done
if ca_no_keychain; then
	falha "a CA continua no keychain do sistema depois do uninstall.sh"
fi
[ -f "$ROOT/var/state.json" ] || falha "o uninstall.sh sem terminal apagou os dados"
[ -f "$CAROOT_DIR/rootCA.pem" ] || falha "o uninstall.sh mexeu nos arquivos da CA do mkcert"

passo "desinstalar apagando os dados"
sh "$REPO/build/darwin/uninstall.sh" --apagar-dados </dev/null || falha "o uninstall.sh --apagar-dados falhou"
[ ! -e "$ROOT" ] || falha "os dados ficaram com --apagar-dados"

passo "ok"
