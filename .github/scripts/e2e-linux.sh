#!/bin/bash
# Teste de ponta a ponta do HyPHP no Linux, rodado pelo job linux-e2e do ci.yml
# num runner ubuntu-24.04 (Homebrew instalado, systemd-resolved, AppArmor, sudo
# sem senha, sem sessão gráfica).
#
# Repete sem ninguém na tela o roteiro manual:
#   - o app instalado pelo .deb do build com o apt-get, que carrega o perfil do
#     AppArmor e registra a action do polkit;
#   - runtimes instalados pelo Homebrew fora do app;
#   - um projeto em ~/Code;
#   - o app aberto num Xvfb, com um D-Bus de sessão só do teste;
#   - as portas 80 e 443, a regra de DNS do .test no systemd-resolved, a
#     confiança na CA e o PATH pelo helper, com o sudo do runner no lugar da
#     janela de senha do pkexec;
#   - checagens pelo curl e pela CLI, o projeto em HTTPS confiado pelo sistema;
#   - o app morto por kill -9 e reaberto, que é o caminho da limpeza de órfãos;
#   - nginx e MariaDB instalados com o app aberto;
#   - um update aplicado pelo atualizador do app, com o mesmo .deb reinstalado
#     pelo pkexec, sem desfazer o que o helper gravou no sistema;
#   - o app fechado e o pacote removido pelo apt-get, que desfaz as mudanças no
#     sistema e mantém os dados, e depois o purge;
#   - o pacote reinstalado e o hosts, o caminho do Linux sem systemd-resolved,
#     pelo helper.
#
# Uso: e2e-linux.sh <caminho do hyphp.deb>
set -euo pipefail

if [ -z "${CI:-}" ]; then
	# Apaga e semeia a pasta de dados do HyPHP e mexe em /etc: numa máquina de
	# verdade isso destruiria o estado do usuário.
	echo "e2e-linux.sh roda só no CI (variável CI ausente)" >&2
	exit 2
fi

DEB=$(cd "$(dirname "${1:?uso: e2e-linux.sh <hyphp.deb>}")" && pwd)/$(basename "$1")
REPO=$(cd "$(dirname "$0")/../.." && pwd)
# O layout do pacote. A CLI vai pelo link /usr/bin/hyphp, como o usuário a
# chama: o app é achado a partir do link resolvido.
MAIN=/usr/lib/hyphp/hyphp
CLI=/usr/bin/hyphp
HELPER=/usr/lib/hyphp/hyphp-helper
ROOT="$HOME/.local/share/HyPHP"
CODE="$HOME/Code"
SITE="http://127.0.0.1"
HOST="Host: teste.test"
ASSUNTO="hyphp-e2e-$RANDOM$RANDOM"
CORPO=$(mktemp)
# A saída do app (GTK, WebKit e o log em stderr), mostrada quando algo falha.
SAIDA_APP=$(mktemp)
# O curl e o openssl do sistema: os do Homebrew, que o shellenv põe na frente,
# leem o pacote de CAs do próprio Homebrew, e não o que o helper atualiza.
CURL=/usr/bin/curl
OPENSSL=/usr/bin/openssl
# Os arquivos que o helper grava no sistema.
DROPIN=/etc/systemd/resolved.conf.d/hyphp.conf
PROFILED=/etc/profile.d/hyphp.sh
ANCORA=/usr/local/share/ca-certificates/hyphp-mkcert.crt
SYSCTLD=/etc/sysctl.d/60-hyphp.conf
# Os arquivos do pacote fora de /usr/lib/hyphp que o teste confere.
POLICY=/usr/share/polkit-1/actions/com.hyphp.app.helper.policy
PERFIL=/etc/apparmor.d/hyphp
# A regra que faz o papel da janela de senha do pkexec no update.
REGRA_POLKIT=/etc/polkit-1/rules.d/49-hyphp-e2e.rules
# A tela e o bus de sessão do teste, derrubados na saída.
XVFB_PID=
BUS_PID=

passo() { printf '\n==> %s\n' "$*"; }

falha() {
	printf '::error::%s\n' "$*"
	exit 1
}

# diagnostico despeja o estado do app e os logs: o ci.yml não guarda
# artefatos, então o log do job é tudo o que sobra para investigar.
diagnostico() {
	echo "::group::diagnóstico"
	"$CLI" status --json || true
	"$CLI" services --json || true
	"$CLI" warnings --json || true
	ss -ltnup || true
	pgrep -fa 'hyphp|httpd|nginx|php-fpm|mysqld|mariadbd|mailpit|Xvfb|dbus-daemon|pkexec|apt-get|dpkg' || true
	ls -la "$ROOT/var/run/procs" || true
	sysctl net.ipv4.ip_unprivileged_port_start kernel.apparmor_restrict_unprivileged_userns || true
	sudo grep -E '^hyphp ' /sys/kernel/security/apparmor/profiles || true
	dpkg-query -W hyphp || true
	resolvectl status || true
	for f in "$DROPIN" "$PROFILED" "$SYSCTLD"; do
		[ -f "$f" ] && echo "--- $f" && cat "$f"
	done
	echo "--- $ROOT/var/update/aplicar.log"
	cat "$ROOT/var/update/aplicar.log" || true
	echo "--- /var/log/apt/term.log (fim)"
	sudo tail -n 60 /var/log/apt/term.log || true
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

# ao_sair despeja o diagnóstico quando o script falha e só depois derruba o
# bus e a tela do teste, de que o app ainda depende durante o diagnóstico.
ao_sair() {
	local status=$?
	if [ "$status" -ne 0 ]; then
		diagnostico
	fi
	[ -z "$BUS_PID" ] || kill "$BUS_PID" 2>/dev/null || true
	[ -z "$XVFB_PID" ] || kill "$XVFB_PID" 2>/dev/null || true
}
trap ao_sair EXIT

# get faz o GET e deixa a resposta em $CORPO. Fora do 200, mostra o fim do
# corpo (onde o PHP escreve o erro) antes de falhar.
get() {
	local codigo
	codigo=$("$CURL" -sS -o "$CORPO" -w '%{http_code}' "$@") || falha "curl falhou: $*"
	if [ "$codigo" != 200 ]; then
		tail -c 2000 "$CORPO"
		echo
		falha "HTTP $codigo em: $*"
	fi
}

# como_app roda um comando no ambiente de uma sessão gráfica sem o shellenv do
# Homebrew, como a do app aberto pelo menu: ele acha o Homebrew sozinho no
# prefixo padrão. O Xvfb não tem GPU, então o GTK e o WebKit desenham sem ela.
# O atualizador também roda assim, porque o app que ele reabre herda o
# ambiente dele.
como_app() {
	env -u HOMEBREW_PREFIX -u HOMEBREW_CELLAR -u HOMEBREW_REPOSITORY PATH="$PATH_SEM_BREW" \
		GSK_RENDERER=cairo WEBKIT_DISABLE_DMABUF_RENDERER=1 "$@"
}

# abrir_app abre o app na tela e no bus do teste. Ninguém nesse bus mostra
# ícones de bandeja: o app se comporta como no GNOME sem a extensão
# AppIndicator.
abrir_app() {
	como_app "$MAIN" >>"$SAIDA_APP" 2>&1 &
}

# app_pid acha o processo do app pela linha de comando exata: o atualizador
# roda o mesmo executável, mas com argumentos.
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

# esperar_aviso espera o app responder com o aviso de código $1.
esperar_aviso() {
	local limite=$((SECONDS + $2))
	while [ "$SECONDS" -lt "$limite" ]; do
		if "$CLI" warnings --json 2>/dev/null | jq -e --arg c "$1" 'any(.[]; .code == $c)' >/dev/null; then
			return 0
		fi
		sleep 2
	done
	falha "o aviso $1 não apareceu em $2 s"
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
# qualquer ordem. Sem argumento, exige nenhum.
conferir_avisos() {
	local esperados
	esperados=$(printf '%s\n' "$@" | jq -R . | jq -sc 'map(select(. != "")) | sort')
	"$CLI" warnings
	"$CLI" warnings --json | jq -e --argjson e "$esperados" '[.[].code] | unique == ($e | unique)' >/dev/null ||
		falha "avisos diferentes de $esperados"
}

# helper roda o hyphp-helper com sudo. O sudo do runner não pede senha e faz o
# papel da janela do pkexec: o helper roda como root, e o SUDO_UID diz quem
# pediu, como o PKEXEC_UID.
helper() {
	local out
	out=$(sudo "$HELPER" "$@") || true
	echo "$out"
	grep -q '"ok":true' <<<"$out" || falha "hyphp-helper $1"
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

# medir_dns exige que um nome de verdade resolva rápido ($1 descreve o
# momento). O DNS= do drop-in vale para o sistema todo: toda consulta passa
# também pelo resolvedor do HyPHP, que responde NXDOMAIN fora do .test, ou por
# nada, com o app fechado. O cache sai antes, para a consulta ir à rede.
medir_dns() {
	local inicio ms
	sudo resolvectl flush-caches
	inicio=$(date +%s%N)
	getent ahosts github.com >/dev/null || falha "github.com não resolveu ($1)"
	ms=$((($(date +%s%N) - inicio) / 1000000))
	echo "github.com resolveu em $ms ms ($1)"
	[ "$ms" -lt 2000 ] || falha "github.com levou $ms ms para resolver ($1)"
}

# folha guarda em $CORPO.leaf o certificado que o web server entrega para
# teste.test.
folha() {
	"$OPENSSL" s_client -connect 127.0.0.1:443 -servername teste.test </dev/null 2>/dev/null |
		"$OPENSSL" x509 >"$CORPO.leaf"
}

# perfil_carregado diz se o kernel tem o perfil hyphp do AppArmor.
perfil_carregado() {
	sudo grep -qE '^hyphp \(' /sys/kernel/security/apparmor/profiles
}

# versao_instalada é o Version do pacote hyphp instalado.
versao_instalada() {
	dpkg-query -W -f="\${Version}" hyphp
}

passo "systemd-resolved no runner"
# O roteiro é o do Ubuntu desktop, em que o primeiro nameserver do resolv.conf
# é o stub do resolved; sem isso o app vai pelo caminho do hosts e os avisos
# são outros. Há imagens do runner com o resolv.conf entregue a outro programa
# (o modo "foreign" do resolved), e aí ele volta ao padrão do Ubuntu.
primeiro_nameserver() { awk '$1 == "nameserver" { print $2; exit }' /etc/resolv.conf; }
cat /etc/resolv.conf
if [ "$(primeiro_nameserver)" != 127.0.0.53 ]; then
	sudo ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf
fi
[ "$(primeiro_nameserver)" = 127.0.0.53 ] || falha "o resolv.conf do runner não aponta para o stub do resolved"
getent ahosts github.com >/dev/null || falha "o runner não resolve nomes pelo stub do resolved"
porta_original=$(sysctl -n net.ipv4.ip_unprivileged_port_start)
echo "limite das portas sem root: $porta_original"

passo "instalar pelo .deb"
# O apt-get instala o arquivo local com as dependências. O postinst carrega o
# perfil do AppArmor: o runner restringe user namespaces como o Ubuntu desktop,
# e sem o perfil o WebKit não isola a página e o app cai ao abrir a janela.
VERSAO=$(dpkg-deb -f "$DEB" Version)
sudo apt-get install -y "$DEB"
for f in "$MAIN" "$HELPER" /usr/lib/hyphp/cli/hyphp; do
	if [ ! -f "$f" ] || [ ! -x "$f" ]; then
		falha "$f não foi instalado como executável"
	fi
done
[ "$(readlink /usr/bin/hyphp)" = /usr/lib/hyphp/cli/hyphp ] || falha "o /usr/bin/hyphp não aponta para a CLI do pacote"
grep -qF '<action id="com.hyphp.app.helper">' "$POLICY" || falha "a action do polkit não é com.hyphp.app.helper"
grep -qF '<annotate key="org.freedesktop.policykit.exec.path">/usr/lib/hyphp/hyphp-helper</annotate>' "$POLICY" ||
	falha "a action do polkit não aponta para o helper do pacote"
[ -f "$PERFIL" ] || falha "o pacote não instalou o $PERFIL"
perfil_carregado || falha "o postinst não carregou o perfil hyphp do AppArmor"
[ "$(versao_instalada)" = "$VERSAO" ] || falha "o pacote instalado não está na versão $VERSAO do .deb"

passo "tela e D-Bus de sessão"
# Uma tela e um bus de sessão para o script todo, como numa sessão gráfica: o
# atualizador reabre o app depois que o antigo sai, e um xvfb-run ou um
# dbus-run-session por abertura derrubaria a tela e o bus junto com o app. O
# SingleInstance do app exige o bus, que nasce com o DISPLAY para os serviços
# que ele ativa (o portal) terem a tela. A tela tem 24 bits de cor, que o GTK4
# usa. O -displayfd escolhe uma tela livre e só a escreve quando ela já aceita
# conexões.
Xvfb -displayfd 3 -screen 0 1920x1080x24 3>"$CORPO.tela" >"$CORPO.xvfb" 2>&1 &
XVFB_PID=$!
for _ in $(seq 50); do
	[ -s "$CORPO.tela" ] && break
	sleep 0.2
done
if [ ! -s "$CORPO.tela" ]; then
	cat "$CORPO.xvfb"
	falha "o Xvfb não abriu a tela"
fi
DISPLAY=:$(<"$CORPO.tela")
export DISPLAY
dbus-daemon --session --fork --print-address=3 --print-pid=4 3>"$CORPO.bus" 4>"$CORPO.bus-pid"
for _ in $(seq 50); do
	[ -s "$CORPO.bus" ] && [ -s "$CORPO.bus-pid" ] && break
	sleep 0.2
done
if [ ! -s "$CORPO.bus" ] || [ ! -s "$CORPO.bus-pid" ]; then
	falha "o dbus-daemon não abriu o bus de sessão"
fi
DBUS_SESSION_BUS_ADDRESS=$(<"$CORPO.bus")
BUS_PID=$(<"$CORPO.bus-pid")
export DBUS_SESSION_BUS_ADDRESS
echo "tela $DISPLAY, bus $DBUS_SESSION_BUS_ADDRESS"

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
"$CURL" -fsSL -o "$CORPO.zip" "$pma_url"
echo "$pma_sha  $CORPO.zip" | sha256sum -c -
mkdir -p "$ROOT/bin/phpmyadmin"
unzip -q "$CORPO.zip" -d "$ROOT/bin/phpmyadmin"

passo "estado inicial e projeto"
# Sem tela para clicar em "Adicionar diretório", a raiz entra direto no
# state.json, onde a tela a grava. O defaultPhp fixo impede que outro PHP da
# imagem do runner vire o PHP do projeto. As portas são as padrão, 80 e 443.
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

passo "abrir o app com as portas baixas presas"
# Com o limite padrão do kernel (1024), o web server não abre a 80 nem a 443:
# o app avisa e oferece liberar.
abrir_app
esperar_aviso ports-pending 180
"$CLI" warnings
fechar_app

passo "liberar as portas baixas pelo helper"
helper ports-write --start 80
[ "$(sysctl -n net.ipv4.ip_unprivileged_port_start)" = 80 ] || falha "o limite das portas não baixou para 80"
cat "$SYSCTLD"

passo "abrir o app"
abrir_app
esperar_pronto 300
"$CLI" status
"$CLI" services
# Sem a regra de DNS e sem a CA, que pedem senha, as duas pendências ficam.
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
	if "$CURL" -fsS http://127.0.0.1:8025/api/v1/messages |
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
helper resolved-write --port 15353
cat "$DROPIN"
# O resolvedor já está no ar: o nome resolve de verdade, sem o cabeçalho Host.
resolvectl query teste.test || true
getent hosts teste.test | grep -q '^127\.0\.0\.1' || falha "teste.test não resolveu pelo resolved"
get "http://teste.test/"
cat "$CORPO"
grep -q '^php=8\.3\.' "$CORPO" || falha "teste.test não abriu pelo nome"
medir_dns "app aberto"
# O passo de usuário do InstallCA: a CA nasce com dono do usuário, sem tocar
# no sistema; só a âncora passa pelo helper.
CAROOT_DIR=$(mkcert -CAROOT)
CAROOT="$CAROOT_DIR" TRUST_STORES=nss mkcert -install
helper ca-trust --cert "$CAROOT_DIR/rootCA.pem"
cmp "$CAROOT_DIR/rootCA.pem" "$ANCORA" || falha "a âncora não é a CA do mkcert"
mkdir -p "$ROOT/cli"
helper profile-write --dir "$ROOT/cli"
cat "$PROFILED"

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
# A reabertura roda um Reconcile completo: agora sem pendência nenhuma.
conferir_avisos
get "$SITE/dados/projetos.json"
jq -e '.projetos[] | select(.domain == "teste.test") | .abre and .https' "$CORPO" >/dev/null ||
	falha "projetos.json não marcou teste.test aberto e em HTTPS"
# O sistema confia no certificado do site: o curl do sistema, sem --cacert, e
# a cadeia conferida contra o pacote de CAs que o update-ca-certificates refez.
get "https://teste.test/"
grep -q '^php=8\.3\.' "$CORPO" || falha "teste.test não respondeu em HTTPS"
folha
"$OPENSSL" verify -CAfile /etc/ssl/certs/ca-certificates.crt "$CORPO.leaf" || falha "o sistema não confia no certificado de teste.test"
# O profile.d põe a pasta cli do HyPHP no PATH do bash de login, atrás do
# /usr/bin, que já tem o hyphp do pacote; o atalho da pasta leva à mesma CLI.
caminhos=$(bash -lc 'printenv PATH' | tail -n 1)
case ":$caminhos:" in
*":$ROOT/cli:"*) ;;
*) falha "o PATH do bash de login não tem $ROOT/cli: $caminhos" ;;
esac
[ "$(readlink -f "$ROOT/cli/hyphp")" = /usr/lib/hyphp/cli/hyphp ] ||
	falha "o atalho $ROOT/cli/hyphp não leva à CLI do pacote"
bash -lc 'hyphp version'

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
get "http://teste.test/"
cat "$CORPO"
grep -q '^php=8\.3\.' "$CORPO" || falha "teste.test não respondeu pelo nginx"
get "https://teste.test/"
grep -q '^php=8\.3\.' "$CORPO" || falha "teste.test não respondeu em HTTPS pelo nginx"
get "$SITE/dados/projetos.json"
jq -e 'any(.projetos[]; .domain == "teste.test")' "$CORPO" >/dev/null || falha "o nginx não serviu o projetos.json"
# Cada motor tem o próprio datadir: o banco do MySQL não existe no MariaDB.
"$CLI" db create hyphp_e2e
get "http://teste.test/db.php"
cat "$CORPO"
grep -q '^mysql=.*MariaDB' "$CORPO" || falha "o PHP não conectou no MariaDB pelo socket"
get "http://127.0.0.1:8036/index.php?route=/server/databases"
grep -q 'hyphp_e2e' "$CORPO" || falha "o phpMyAdmin pelo nginx não listou o banco do MariaDB"

passo "update aplicado pelo atualizador"
# O pedido que o Launch do app monta (internal/update/apply_linux.go): o .deb
# baixado em var/update/<versão>, com o latest.json e a assinatura ao lado,
# conferido por tamanho e sha256, e o app instalado como atualizador. O mesmo
# .deb faz o papel da versão nova: o helper o reinstala, e o dpkg roda os
# scripts de upgrade como numa versão maior. O app sai pelo SIGTERM, que cai no
# mesmo app.Quit que o Launch chama; o atualizador roda o install-update do
# helper pelo pkexec e reabre o app.
#
# A regra do polkit faz o papel da janela de senha, como o sudo faz para o
# helper no resto do roteiro: com a autorização dada pela regra, o pkexec não
# precisa de agente de autenticação.
sudo mkdir -p "$(dirname "$REGRA_POLKIT")"
sudo tee "$REGRA_POLKIT" >/dev/null <<EOF
polkit.addRule(function (action, subject) {
	if (action.id == "com.hyphp.app.helper" && subject.user == "$(id -un)") {
		return polkit.Result.YES;
	}
});
EOF
UPD="$ROOT/var/update"
BAIXADO="$UPD/$VERSAO/hyphp-$VERSAO-linux-amd64.deb"
mkdir -p "$UPD/$VERSAO"
cp "$DEB" "$BAIXADO"
SHA=$(sha256sum "$BAIXADO" | cut -d ' ' -f 1)
TAM=$(stat -c %s "$BAIXADO")
# O install-update só instala o pacote que um latest.json assinado descreve. O
# CI não tem a chave das releases: o manifesto é assinado com uma chave de
# teste, e o helper do pacote dá lugar a um compilado com a pública dela
# (internal/update/key.go). A reinstalação devolve o helper do pacote.
CHAVE="$CORPO.chave"
"$OPENSSL" genpkey -algorithm ed25519 -out "$CHAVE"
publica=$("$OPENSSL" pkey -in "$CHAVE" -pubout -outform DER | tail -c 32 | od -An -v -tx1 | tr -d ' \n')
zeros=$(printf '0%.0s' $(seq 64))
jq -n --arg v "$VERSAO" --arg data "$(date +%F)" --arg deb "$(basename "$BAIXADO")" \
	--arg sha "$SHA" --argjson tam "$TAM" --arg zeros "$zeros" \
	'{schema: 1, version: $v, date: $data, notes: ["e2e"],
	  windows_amd64: {path: "hyphp-\($v)-windows-amd64-setup.exe", size: 1, sha256: $zeros},
	  linux_amd64: {path: $deb, size: $tam, sha256: $sha}}' >"$UPD/$VERSAO/latest.json"
"$OPENSSL" pkeyutl -sign -inkey "$CHAVE" -rawin -in "$UPD/$VERSAO/latest.json" | base64 -w0 >"$UPD/$VERSAO/latest.json.sig"
(cd "$REPO" && CGO_ENABLED=0 go build -trimpath -ldflags "-X hyphp/internal/update.publicKeyHex=$publica" \
	-o "$CORPO.helper" ./cmd/hyphp-helper)
sudo install -m 0755 "$CORPO.helper" "$HELPER"
# O dpkg grava cada arquivo num inode novo: o mesmo inode depois do update quer
# dizer que o pacote não foi reinstalado.
inode_antes=$(stat -c %i "$MAIN")
pid_antigo=$(app_pid)
como_app "$MAIN" --aplicar-update --pid "$pid_antigo" --instalador "$BAIXADO" \
	--sha256 "$SHA" --tamanho "$TAM" --dir /usr/lib/hyphp --exe hyphp \
	--resultado "$UPD/resultado.json" --de 0.0.0 --para "$VERSAO" --idioma en >>"$SAIDA_APP" 2>&1 &
atualizador=$!
pkill -TERM -f "^$MAIN\$"
status_atualizador=0
wait "$atualizador" || status_atualizador=$?
sudo rm -f "$REGRA_POLKIT"
cat "$UPD/aplicar.log" || true
[ "$status_atualizador" -eq 0 ] || falha "o atualizador terminou com erro ($status_atualizador)"
grep -q 'resultado: ok=true' "$UPD/aplicar.log" || falha "o atualizador não registrou sucesso"
[ "$(versao_instalada)" = "$VERSAO" ] || falha "o pacote não está na versão $VERSAO depois do update"
[ "$(stat -c %i "$MAIN")" != "$inode_antes" ] || falha "o update não reinstalou o pacote"
if cmp -s "$CORPO.helper" "$HELPER"; then
	falha "o helper de teste ficou no lugar do helper do pacote"
fi
# No upgrade o prerm não desfaz o que o helper gravou no sistema, e o postinst
# recarrega o perfil.
for f in "$DROPIN" "$PROFILED" "$ANCORA" "$SYSCTLD"; do
	[ -e "$f" ] || falha "$f sumiu no update"
done
[ "$(sysctl -n net.ipv4.ip_unprivileged_port_start)" = 80 ] || falha "o limite das portas mudou no update"
perfil_carregado || falha "o perfil hyphp do AppArmor saiu do kernel no update"
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
medir_dns "app fechado, regra de DNS no lugar"

passo "remover o pacote pelo apt-get"
# Com o app fechado, o prerm desfaz pelo helper o que ele gravou no sistema e
# tira o perfil do kernel; os dados do usuário ficam.
sudo apt-get remove -y hyphp
for f in "$DROPIN" "$PROFILED" "$ANCORA" "$SYSCTLD"; do
	[ ! -e "$f" ] || falha "$f ficou depois do apt-get remove"
done
[ "$(sysctl -n net.ipv4.ip_unprivileged_port_start)" = "$porta_original" ] ||
	falha "o limite das portas não voltou para $porta_original"
if "$OPENSSL" verify -CAfile /etc/ssl/certs/ca-certificates.crt "$CORPO.leaf"; then
	falha "o sistema ainda confia no certificado do site"
fi
if getent hosts teste.test; then
	falha "teste.test ainda resolve sem a regra de DNS"
fi
medir_dns "sem a regra de DNS"
[ ! -e /usr/lib/hyphp ] || falha "o /usr/lib/hyphp ficou depois do apt-get remove"
if [ -e /usr/bin/hyphp ] || [ -L /usr/bin/hyphp ]; then
	falha "o /usr/bin/hyphp ficou depois do apt-get remove"
fi
[ ! -e "$PERFIL" ] || falha "o $PERFIL ficou depois do apt-get remove"
if perfil_carregado; then
	falha "o perfil hyphp do AppArmor ficou no kernel"
fi
[ ! -e "$POLICY" ] || falha "a action do polkit ficou depois do apt-get remove"
[ -f "$ROOT/var/state.json" ] || falha "o apt-get remove apagou os dados do usuário"
sudo apt-get purge -y hyphp
[ ! -e /var/cache/hyphp ] || falha "o /var/cache/hyphp ficou depois do purge"

passo "reinstalar pelo .deb"
sudo apt-get install -y "$DEB"
perfil_carregado || falha "o postinst não carregou o perfil hyphp do AppArmor"

passo "reabrir sem as mudanças no sistema"
abrir_app
esperar_aviso ports-pending 180
conferir_avisos ca-pending wildcard-pending ports-pending
fechar_app

passo "hosts pelo helper, o caminho sem systemd-resolved"
# O runner tem o resolved, então o app não grava o hosts sozinho: o helper
# grava o bloco que o app montaria, e o uninstall o tira sem mexer no resto.
cp /etc/hosts "$CORPO.hosts-original"
{
	cat /etc/hosts
	printf '\n# hyphp:start\n127.0.0.1 teste.test\n# hyphp:end\n'
} >"$CORPO.hosts-next"
helper hosts-write --from "$CORPO.hosts-next"
getent hosts teste.test | grep -q '^127\.0\.0\.1' || falha "teste.test não resolveu pelo hosts"
helper uninstall
cmp "$CORPO.hosts-original" /etc/hosts || falha "o uninstall não devolveu o hosts como estava"

passo "ok"
