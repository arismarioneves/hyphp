#!/bin/bash
# Teste de ponta a ponta do HyPHP no macOS, rodado pelo job macos-e2e do ci.yml
# num runner macos-15 (Apple Silicon, sessão gráfica, Homebrew instalado).
#
# Repete sem ninguém na tela o roteiro manual da M1 e da M2:
#   - runtimes instalados pelo Homebrew fora do app;
#   - um projeto em ~/Code;
#   - o .app aberto pelo `open`;
#   - checagens pelo curl e pela CLI do bundle;
#   - a regra de DNS, a confiança na CA e o PATH registrados pelo helper, com o
#     sudo do runner no lugar da janela de senha;
#   - o projeto em HTTPS, confiado pelo sistema;
#   - o app morto por kill -9 e reaberto, que é o caminho da limpeza de órfãos;
#   - o app fechado normalmente e as mudanças removidas do sistema.
#
# Escrito para o bash 3.2 do macOS: sem arrays vazios sob `set -u`.
#
# Uso: e2e-macos.sh <caminho do HyPHP.app>
set -euo pipefail

if [ -z "${CI:-}" ]; then
	# Apaga e semeia a pasta de dados do HyPHP: num Mac de verdade isso
	# destruiria o estado do usuário.
	echo "e2e-macos.sh roda só no CI (variável CI ausente)" >&2
	exit 2
fi

APP=$(cd "${1:?uso: e2e-macos.sh <HyPHP.app>}" && pwd)
REPO=$(cd "$(dirname "$0")/../.." && pwd)
CLI="$APP/Contents/Helpers/hyphp"
MAIN="$APP/Contents/MacOS/hyphp"
ROOT="$HOME/Library/Application Support/HyPHP"
CODE="$HOME/Code"
SITE="http://127.0.0.1"
HOST="Host: teste.test"
ASSUNTO="hyphp-e2e-$RANDOM$RANDOM"
CORPO=$(mktemp)

passo() { printf '\n==> %s\n' "$*"; }
falha() {
	printf '::error::%s\n' "$*"
	exit 1
}

# diagnostico despeja o estado do app e os logs quando o script falha: o ci.yml
# não guarda artefatos, então o log do job é tudo o que sobra para investigar.
diagnostico() {
	local status=$?
	[ "$status" -eq 0 ] && return
	echo "::group::diagnóstico"
	"$CLI" status --json || true
	"$CLI" services --json || true
	"$CLI" warnings --json || true
	lsof -nP -iTCP -sTCP:LISTEN || true
	pgrep -fl 'hyphp|httpd|php-fpm|mysqld|mailpit' || true
	ls -la "$ROOT/var/run/procs" || true
	for f in "$ROOT"/log/*.log; do
		[ -f "$f" ] || continue
		echo "--- $f"
		tail -n 80 "$f"
	done
	echo "::endgroup::"
}
trap diagnostico EXIT

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

passo "abrir o app"
open "$APP"
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
# Cada motor tem o próprio datadir: o banco do MySQL não existe no MariaDB.
"$CLI" db create hyphp_e2e
get -H "$HOST" "$SITE/db.php"
cat "$CORPO"
grep -q '^mysql=.*MariaDB' "$CORPO" || falha "o PHP não conectou no MariaDB pelo socket"
get "http://127.0.0.1:8036/index.php?route=/server/databases"
grep -q 'hyphp_e2e' "$CORPO" || falha "o phpMyAdmin pelo nginx não listou o banco do MariaDB"

passo "fechar o app"
fechar_app

passo "remover do sistema pelo helper"
helper uninstall --cert "$CAROOT_DIR/rootCA.pem"
[ ! -e /etc/resolver/test ] || falha "a regra de DNS ficou"
[ ! -e /etc/paths.d/hyphp ] || falha "o /etc/paths.d/hyphp ficou"
# A CA tem de sumir do keychain do sistema, conferida pelo SHA-1 como o
# CAInstalled do app faz: o verify-cert na raiz seguiu dando 0 depois da
# remoção no spike, então só a folha serve para conferir a confiança.
sha1=$(openssl x509 -in "$CAROOT_DIR/rootCA.pem" -noout -fingerprint -sha1 | cut -d= -f2 | tr -d :)
# A listagem vem antes do grep: no pipe, o grep -q sai no primeiro acerto, o
# security leva SIGPIPE (141) e, com pipefail, o if daria falso justo quando a
# CA ficou.
listagem=$(security find-certificate -a -Z /Library/Keychains/System.keychain)
if grep -q "SHA-1 hash: $sha1" <<<"$listagem"; then
	falha "a CA continua no keychain do sistema"
fi
if security verify-cert -c "$CORPO.leaf" -p ssl -s teste.test; then
	falha "o sistema ainda confia no certificado do site"
fi

passo "reabrir sem as mudanças no sistema"
open "$APP"
esperar_pronto 300
conferir_avisos ca-pending wildcard-pending

passo "fechar o app de novo"
fechar_app

passo "ok"
