#!/bin/sh
# Instala ou atualiza o HyPHP no Mac (Apple Silicon, macOS 15+):
#
#   curl -fsSL https://github.com/arismarioneves/hyphp/releases/latest/download/install.sh | sh
#
# Baixa o dmg da release mais nova, confere o sha256 anunciado no latest.json,
# fecha o HyPHP se estiver aberto e troca /Applications/HyPHP.app. O curl não
# marca o arquivo com quarentena, então o Gatekeeper não barra o app assinado
# ad-hoc, como faria com o dmg baixado pelo navegador. HYPHP_URL_BASE troca a
# pasta de onde tudo é baixado (o teste de ponta a ponta usa um servidor
# local). A assinatura ed25519 do latest.json fica com o app, no update: aqui
# a integridade vem do HTTPS do GitHub.
set -eu

BASE=${HYPHP_URL_BASE:-https://github.com/arismarioneves/hyphp/releases/latest/download}
DESTINO=/Applications/HyPHP.app
EXE="$DESTINO/Contents/MacOS/hyphp"

# Os scripts não têm i18n: cada mensagem sai em português e em inglês.
diz() { printf '%s\n  %s\n' "$1" "$2"; }
falha() {
	printf 'erro: %s\n  error: %s\n' "$1" "$2" >&2
	exit 1
}

[ "$(uname -s)" = Darwin ] || falha "este script é só para macOS" "this script is for macOS only"
[ "$(uname -m)" = arm64 ] || falha "o HyPHP para Mac exige Apple Silicon" "HyPHP for Mac requires Apple Silicon"
versao_mac=$(sw_vers -productVersion)
[ "${versao_mac%%.*}" -ge 15 ] ||
	falha "o HyPHP exige macOS 15 ou mais novo (este é o $versao_mac)" "HyPHP requires macOS 15 or later (this is $versao_mac)"

tmp=$(mktemp -d)
montado=""
limpar() {
	if [ -n "$montado" ]; then
		hdiutil detach -quiet -force "$tmp/volume" || true
	fi
	rm -rf "$tmp"
}
trap limpar EXIT
trap 'exit 1' INT TERM

diz "Baixando o latest.json de $BASE" "Downloading latest.json from $BASE"
curl -fsSL -o "$tmp/latest.json" "$BASE/latest.json" ||
	falha "não consegui baixar $BASE/latest.json" "could not download $BASE/latest.json"
# O plutil lê JSON e vem com o macOS: nada a instalar antes.
campo() {
	plutil -extract "$1" raw -o - "$tmp/latest.json" 2>/dev/null ||
		falha "o latest.json não tem $1" "latest.json has no $1"
}
versao=$(campo version)
nome=$(campo darwin_arm64.path)
sha=$(campo darwin_arm64.sha256)
# O nome vira URL e caminho: só um nome de arquivo simples.
case $nome in
"" | */* | .*) falha "nome de arquivo inválido no latest.json: $nome" "invalid file name in latest.json: $nome" ;;
esac

diz "Baixando o HyPHP $versao ($nome)" "Downloading HyPHP $versao ($nome)"
curl -fSL --progress-bar -o "$tmp/hyphp.dmg" "$BASE/$nome" ||
	falha "não consegui baixar $BASE/$nome" "could not download $BASE/$nome"
obtido=$(shasum -a 256 "$tmp/hyphp.dmg" | cut -d ' ' -f 1)
[ "$obtido" = "$sha" ] ||
	falha "o sha256 do dmg ($obtido) não é o anunciado ($sha)" "the dmg sha256 ($obtido) does not match the announced one ($sha)"

mkdir "$tmp/volume"
hdiutil attach -nobrowse -readonly -noautoopen -mountpoint "$tmp/volume" "$tmp/hyphp.dmg" >/dev/null ||
	falha "não consegui montar o dmg" "could not mount the dmg"
montado=1
[ -d "$tmp/volume/HyPHP.app" ] || falha "o dmg não tem o HyPHP.app" "the dmg has no HyPHP.app"

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

# como_admin roda direto se /Applications aceita gravação (conta de
# administrador) e pelo sudo se não; o sudo pede a senha no terminal mesmo com
# o script vindo pelo pipe do curl.
como_admin() {
	if [ -w /Applications ]; then
		"$@"
	else
		sudo "$@"
	fi
}

fechar_app

# A mesma troca do update pelo app (internal/update/apply_darwin.go): cópia ao
# lado, dois renames e o app anterior de volta se o segundo falhar. Um erro no
# meio devolve o app anterior; se nem isso der certo, o script diz onde ele ficou.
novo=/Applications/.HyPHP.app.novo
antigo=/Applications/.HyPHP.app.antigo
como_admin rm -rf "$novo" "$antigo"
como_admin ditto "$tmp/volume/HyPHP.app" "$novo" || falha "não consegui copiar o app" "could not copy the app"
if [ -e "$DESTINO" ]; then
	como_admin mv "$DESTINO" "$antigo" || falha "não consegui afastar o app anterior" "could not move the previous app aside"
fi
if ! como_admin mv "$novo" "$DESTINO"; then
	como_admin rm -rf "$novo" || true
	if [ -e "$antigo" ]; then
		como_admin mv "$antigo" "$DESTINO" ||
			falha "não consegui instalar o app novo nem devolver o anterior, que ficou em $antigo" "could not install the new app nor restore the previous one, left at $antigo"
		falha "não consegui instalar o app novo; o anterior ficou" "could not install the new app; the previous one was kept"
	fi
	falha "não consegui instalar o app novo" "could not install the new app"
fi
como_admin rm -rf "$antigo" || diz "Sobrou $antigo; pode apagar" "$antigo was left behind; you can delete it"
hdiutil detach -quiet "$tmp/volume" && montado=""

diz "HyPHP $versao instalado em $DESTINO" "HyPHP $versao installed in $DESTINO"
open "$DESTINO"
