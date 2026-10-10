#!/bin/sh
# Remove o HyPHP do Mac:
#
#   curl -fsSL https://github.com/arismarioneves/hyphp/releases/latest/download/uninstall.sh | sh
#   curl -fsSL …/uninstall.sh | sh -s -- --apagar-dados
#
# Fecha o app, desfaz pelo hyphp-helper do bundle o que ele gravou no sistema
# (a regra de DNS .test, o /etc/paths.d/hyphp e a confiança na CA do mkcert),
# apaga o início automático, a pasta cli e o app. Os dados (bancos,
# configurações, logs) ficam, a menos que o usuário confirme no terminal ou
# passe --apagar-dados. O Homebrew, as fórmulas e os arquivos da CA do mkcert
# nunca são tocados.
set -eu

APP=/Applications/HyPHP.app
EXE="$APP/Contents/MacOS/hyphp"
HELPER="$APP/Contents/Helpers/hyphp-helper"
DADOS="$HOME/Library/Application Support/HyPHP"
AGENTE="$HOME/Library/LaunchAgents/com.hyphp.app.plist"

# Os scripts não têm i18n: cada mensagem sai em português e em inglês.
diz() { printf '%s\n  %s\n' "$1" "$2"; }
falha() {
	printf 'erro: %s\n  error: %s\n' "$1" "$2" >&2
	exit 1
}

apagar=""
for arg in "$@"; do
	case $arg in
	--apagar-dados) apagar=1 ;;
	*) falha "opção desconhecida: $arg" "unknown option: $arg" ;;
	esac
done

# fechar_app manda SIGTERM ao HyPHP aberto, o mesmo encerramento do menu Sair
# (os serviços param), e espera até 60 s.
fechar_app() {
	pgrep -f "$EXE" >/dev/null || return 0
	diz "Fechando o HyPHP aberto" "Closing the running HyPHP"
	pkill -TERM -f "$EXE" || true
	i=0
	while pgrep -f "$EXE" >/dev/null; do
		i=$((i + 1))
		[ "$i" -le 60 ] || falha "o HyPHP não fechou em 60 s" "HyPHP did not quit within 60 s"
		sleep 1
	done
}

fechar_app

# O helper fica dentro do bundle: as mudanças no sistema saem antes do app.
if [ -x "$HELPER" ]; then
	# O CAROOT é o que o app usa: o do mkcert, quando ele existe.
	mkcert=$(command -v mkcert || true)
	if [ -z "$mkcert" ] && [ -x /opt/homebrew/bin/mkcert ]; then
		mkcert=/opt/homebrew/bin/mkcert
	fi
	if [ -n "$mkcert" ]; then
		caroot=$("$mkcert" -CAROOT)
	else
		caroot="$HOME/Library/Application Support/mkcert"
	fi
	set -- uninstall
	if [ -f "$caroot/rootCA.pem" ]; then
		set -- uninstall --cert "$caroot/rootCA.pem"
	fi
	diz "Desfazendo as mudanças no sistema (pede a senha de administrador)" "Undoing the system changes (asks for the administrator password)"
	saida=$(sudo "$HELPER" "$@") || falha "o hyphp-helper falhou: $saida" "hyphp-helper failed: $saida"
else
	diz "$APP não encontrado: a regra de DNS, o PATH e a CA ficaram no sistema" "$APP not found: the DNS rule, PATH entry and CA were left in the system"
fi

rm -f "$AGENTE"
rm -rf "$DADOS/cli"
if [ -e "$APP" ]; then
	if [ -w /Applications ]; then
		rm -rf "$APP"
	else
		sudo rm -rf "$APP"
	fi
fi
diz "HyPHP removido" "HyPHP removed"

# Sem terminal (o e2e, um script) não há a quem perguntar, e os dados ficam.
# O /dev/tty, e não o stdin: no `curl | sh` o stdin é o próprio script.
if [ -z "$apagar" ] && [ -d "$DADOS" ] && (exec </dev/tty) 2>/dev/null; then
	printf 'Apagar também os dados (bancos, configurações, logs) em %s? / Also delete the data (databases, settings, logs)? [s/N] ' "$DADOS" >/dev/tty
	read -r resposta </dev/tty || resposta=""
	case $resposta in
	s | S | y | Y) apagar=1 ;;
	esac
fi
if [ -n "$apagar" ]; then
	rm -rf "$DADOS"
	diz "Dados apagados: $DADOS" "Data deleted: $DADOS"
elif [ -d "$DADOS" ]; then
	diz "Os dados ficaram em $DADOS" "The data was kept in $DADOS"
fi
