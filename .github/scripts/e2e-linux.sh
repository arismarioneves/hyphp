#!/bin/bash
# Teste de ponta a ponta do HyPHP no Linux, rodado pelo job linux-e2e do ci.yml
# num runner ubuntu-24.04 (Homebrew instalado, sem sessão gráfica).
#
# Repete sem ninguém na tela o roteiro manual:
#   - runtimes instalados pelo Homebrew fora do app;
#   - um projeto em ~/Code;
#   - o app do build local (bin/hyphp, com a CLI em bin/cli/hyphp) num Xvfb,
#     com um D-Bus de sessão só dele;
#   - checagens pelo curl e pela CLI;
#   - o app morto por kill -9 e reaberto, que é o caminho da limpeza de órfãos;
#   - nginx e MariaDB instalados com o app aberto;
#   - o app fechado normalmente.
#
# As portas 80 e 443, o hosts, a CA e o PATH pedem privilégio de administrador
# e ficam de fora: o site responde na 8080, pelo cabeçalho Host.
#
# Uso: e2e-linux.sh, depois do `wails3 task linux:build` e do
# `wails3 task linux:apparmor`
set -euo pipefail

if [ -z "${CI:-}" ]; then
	# Apaga e semeia a pasta de dados do HyPHP: numa máquina de verdade isso
	# destruiria o estado do usuário.
	echo "e2e-linux.sh roda só no CI (variável CI ausente)" >&2
	exit 2
fi

REPO=$(cd "$(dirname "$0")/../.." && pwd)
MAIN="$REPO/bin/hyphp"
CLI="$REPO/bin/cli/hyphp"
ROOT="$HOME/.local/share/HyPHP"
CODE="$HOME/Code"
SITE="http://127.0.0.1:8080"
HOST="Host: teste.test"
ASSUNTO="hyphp-e2e-$RANDOM$RANDOM"
CORPO=$(mktemp)
# A saída do app (GTK, WebKit e o log em stderr), mostrada quando algo falha.
SAIDA_APP=$(mktemp)

passo() { printf '\n==> %s\n' "$*"; }

falha() {
	printf '::error::%s\n' "$*"
	exit 1
}

# ao_sair despeja o estado do app e os logs quando o script falha: o ci.yml não
# guarda artefatos, então o log do job é tudo o que sobra para investigar.
ao_sair() {
	local status=$?
	[ "$status" -eq 0 ] && return
	echo "::group::diagnóstico"
	"$CLI" status --json || true
	"$CLI" services --json || true
	"$CLI" warnings --json || true
	ss -ltnp || true
	pgrep -fa 'hyphp|httpd|nginx|php-fpm|mysqld|mariadbd|mailpit|Xvfb|dbus-daemon' || true
	ls -la "$ROOT/var/run/procs" || true
	# O WebKitGTK isola a página com o bwrap, que só cria user namespaces com o
	# perfil do AppArmor carregado onde o sistema os restringe.
	sysctl kernel.apparmor_restrict_unprivileged_userns || true
	grep -F hyphp-dev /sys/kernel/security/apparmor/profiles || true
	# Num crash do Go, o motivo vem na linha do sinal ou do fatal error, e o
	# que o GTK ou o WebKit escreveu vem logo antes dela.
	echo "--- saída do app (motivo do fim)"
	grep -v -E 'xdg-desktop-portal|^$' "$SAIDA_APP" | grep -m 1 -B 25 -A 6 -E '^(SIG[A-Z]+|fatal error|panic):' || true
	echo "--- saída do app (começo)"
	head -n 40 "$SAIDA_APP" || true
	for f in "$ROOT"/log/*.log; do
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

# abrir_app abre o app como a sessão gráfica abriria, num Xvfb e com um D-Bus
# de sessão próprio, que o SingleInstance exige; o bus nasce dentro do Xvfb
# para os serviços que ele ativa (o portal) terem a tela. Ninguém nesse bus
# mostra ícones de bandeja: o app se comporta como no GNOME sem a extensão
# AppIndicator. O ambiente é o de uma sessão sem o shellenv do Homebrew, como
# a do app aberto pelo menu: ele acha o Homebrew sozinho no prefixo padrão. A
# tela padrão do xvfb-run tem 8 bits de cor, que o GTK4 não usa; e o Xvfb não
# tem GPU, então o GTK e o WebKit desenham sem ela.
abrir_app() {
	env -u HOMEBREW_PREFIX -u HOMEBREW_CELLAR -u HOMEBREW_REPOSITORY PATH="$PATH_SEM_BREW" \
		GSK_RENDERER=cairo WEBKIT_DISABLE_DMABUF_RENDERER=1 \
		xvfb-run -a -s "-screen 0 1920x1080x24" dbus-run-session -- "$MAIN" >>"$SAIDA_APP" 2>&1 &
}

# app_pid acha o processo do app pela linha de comando exata: o xvfb-run e o
# dbus-run-session também a citam, mas com mais palavras.
app_pid() {
	pgrep -f "^$MAIN\$"
}

# esperar_pronto espera a CLI responder com todos os serviços prontos e os
# quatro grupos do roteiro presentes. Antes disso o socket pode nem existir.
# Um app que morreu não fica pronto: depois de ele ter tido tempo de abrir, a
# espera acaba na hora.
esperar_pronto() {
	local inicio=$SECONDS limite=$((SECONDS + $1)) s
	while [ "$SECONDS" -lt "$limite" ]; do
		if s=$("$CLI" services --json 2>/dev/null) &&
			jq -e 'length > 0 and all(.[]; .state == "ready")
				and ([.[].group] | contains(["web", "php", "db", "mail"]))' <<<"$s" >/dev/null; then
			return 0
		fi
		if [ $((SECONDS - inicio)) -gt 15 ] && ! app_pid >/dev/null; then
			falha "o app terminou antes de os serviços ficarem prontos"
		fi
		sleep 3
	done
	falha "serviços não ficaram prontos em $1 s"
}

# esperar_saida espera o processo principal do app terminar.
esperar_saida() {
	local limite=$((SECONDS + $1))
	while app_pid >/dev/null; do
		[ "$SECONDS" -lt "$limite" ] || falha "o app não terminou em $1 s"
		sleep 1
	done
}

# pids lista "pid id" de cada serviço com processo.
pids() {
	"$CLI" services --json | jq -r '.[] | select(.pid > 0) | "\(.pid) \(.id)"'
}

# conferir_avisos mostra os avisos e exige exatamente os códigos dados, em
# qualquer ordem.
conferir_avisos() {
	local esperados
	esperados=$(printf '%s\n' "$@" | jq -R . | jq -sc 'map(select(. != "")) | sort')
	"$CLI" warnings
	"$CLI" warnings --json | jq -e --argjson e "$esperados" '[.[].code] | unique == ($e | unique)' >/dev/null ||
		falha "avisos diferentes de $esperados"
}

# fechar_app fecha pelo mesmo caminho do menu Sair e exige que nenhum serviço
# nem registro de processo sobre.
fechar_app() {
	local depois
	depois=$(pids)
	# SIGTERM cai no mesmo app.Quit do menu Sair (handler de sinal do Wails).
	pkill -TERM -f "^$MAIN\$"
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

# instalar roda o `brew install` como o app (internal/brew/exec_unix.go): uma
# fórmula por vez, o PHP pelo nome completo do tap, sem tap/trust, e a saída 1
# com o keg no opt/ conta como sucesso, porque só o link em bin/ falhou (por
# exemplo, mariadb@11.8 ao lado do mysql@8.4).
instalar() {
	brew install "$1" || [ -d "$(brew --prefix)/opt/${1##*/}" ] || falha "brew install $1"
}

passo "runtimes pelo Homebrew"
# O Homebrew do runner fica fora do PATH; o shellenv vale só para o script.
PATH_SEM_BREW=$PATH
eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
export HOMEBREW_NO_ENV_HINTS=1 HOMEBREW_NO_COLOR=1 HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1
for f in shivammathur/php/php@8.3 httpd mysql@8.4 mailpit mkcert; do
	instalar "$f"
done
ls -ld "$(brew --prefix)"/opt/{php*,httpd,mysql*,mariadb*,mailpit,mkcert,nginx} 2>/dev/null || true

passo "phpMyAdmin do catálogo"
# O app baixa o phpMyAdmin pela tela Runtimes, que o teste não alcança. Aqui vai
# o mesmo zip do catálogo, conferido pelo mesmo sha256, para a pasta onde o
# pkgmgr o colocaria (bin/phpmyadmin/<raiz do zip>).
catalogo="$REPO/internal/pkgmgr/catalog.json"
pma_url=$(jq -r '.packages[] | select(.kind == "phpmyadmin") | .url' "$catalogo")
pma_sha=$(jq -r '.packages[] | select(.kind == "phpmyadmin") | .sha256' "$catalogo")
curl -fsSL -o "$CORPO.zip" "$pma_url"
echo "$pma_sha  $CORPO.zip" | sha256sum -c -
mkdir -p "$ROOT/bin/phpmyadmin"
unzip -q "$CORPO.zip" -d "$ROOT/bin/phpmyadmin"

passo "estado inicial e projeto"
# Sem tela para clicar em "Adicionar diretório", a raiz entra direto no
# state.json, onde a tela a grava. O defaultPhp fixo impede que outro PHP da
# imagem do runner vire o PHP do projeto. Sem privilégio, o web server não
# abre a 80 nem a 443.
mkdir -p "$ROOT/var" "$CODE/teste"
cat >"$ROOT/var/state.json" <<EOF
{"schemaVersion": 1, "defaultPhp": "8.3", "roots": ["$CODE"], "httpPort": 8080, "httpsPort": 8443}
EOF
cat >"$CODE/teste/index.php" <<'EOF'
<?php
header('Content-Type: text/plain');
echo 'php=', PHP_VERSION, "\n";
EOF
# "localhost" faz o mysqli usar o socket padrão, o que confere o
# mysqli.default_socket que o HyPHP grava no php.ini.
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
ss -ltnp || true

passo "abrir o app"
abrir_app
esperar_pronto 300
"$CLI" status
"$CLI" services
# Sem o hosts e sem a CA, que pedem senha, as duas pendências ficam.
conferir_avisos ca-pending hosts-pending
"$CLI" status --json | jq -e '.failed == 0 and .ready == .total' >/dev/null ||
	falha "status com serviço em falha"

passo "projeto pelo Apache na porta 8080"
get -H "$HOST" "$SITE/"
cat "$CORPO"
grep -q '^php=8\.3\.' "$CORPO" || falha "teste.test não respondeu com o PHP 8.3"
# A página de host sem projeto lista os projetos para esta máquina: teste.test
# não está no hosts e só abre com o cabeçalho Host.
get "$SITE/dados/projetos.json"
cat "$CORPO"
jq -e '.projetos[] | select(.domain == "teste.test") | .abre == false and .motivo == "hosts"' "$CORPO" >/dev/null ||
	falha "projetos.json sem teste.test pendente do hosts"
get -H "Host: nada.test" "$SITE/"
grep -q 'id="textos"' "$CORPO" || falha "host sem projeto não serviu a página do HyPHP"
grep -qF '<code>~/Code</code>' "$CORPO" || falha "a página do HyPHP não deu a pasta de exemplo do Linux"

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

passo "kill -9 no app"
antes=$(pids)
echo "$antes"
ls -l "$ROOT/var/run/procs"
pkill -9 -f "^$MAIN\$"
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
abrir_app
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
conferir_avisos ca-pending hosts-pending

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
conferir_avisos ca-pending hosts-pending

passo "projeto e banco pelo nginx e MariaDB"
get -H "$HOST" "$SITE/"
cat "$CORPO"
grep -q '^php=8\.3\.' "$CORPO" || falha "teste.test não respondeu pelo nginx"
get "$SITE/dados/projetos.json"
jq -e 'any(.projetos[]; .domain == "teste.test")' "$CORPO" >/dev/null || falha "o nginx não serviu o projetos.json"
# Cada motor tem o próprio datadir: o banco do MySQL não existe no MariaDB.
"$CLI" db create hyphp_e2e
get -H "$HOST" "$SITE/db.php"
cat "$CORPO"
grep -q '^mysql=.*MariaDB' "$CORPO" || falha "o PHP não conectou no MariaDB pelo socket"
get "http://127.0.0.1:8036/index.php?route=/server/databases"
grep -q 'hyphp_e2e' "$CORPO" || falha "o phpMyAdmin pelo nginx não listou o banco do MariaDB"

passo "fechar o app"
fechar_app

passo "ok"
