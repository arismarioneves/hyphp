#!/bin/sh
set -e

# Só na remoção: no upgrade o app novo segue usando o que o helper gravou no
# sistema, e o updater ainda está rodando.
if [ "$1" = "remove" ]; then
    # Fecha o app e a bandeja, nunca o updater (--aplicar-update), que não
    # casa com o padrão.
    app='^/usr/lib/hyphp/hyphp( --bandeja)?$'
    if pkill -TERM -f "$app" 2>/dev/null; then
        i=0
        while [ "$i" -lt 30 ] && pgrep -f "$app" >/dev/null 2>&1; do
            sleep 1
            i=$((i + 1))
        done
    fi

    # Uma saída diferente de zero aqui aborta o apt remove: avisa e segue. O
    # motivo vem no JSON que o helper escreve no stdout.
    if ! saida=$(/usr/lib/hyphp/hyphp-helper uninstall); then
        echo "hyphp: could not undo the system changes made by HyPHP (hyphp-helper uninstall): $saida" >&2
    fi

    # O apparmor_parser -R precisa do arquivo, que o dpkg apaga logo depois.
    if aa-enabled --quiet 2>/dev/null; then
        apparmor_parser -R /etc/apparmor.d/hyphp 2>/dev/null || true
    fi
fi

exit 0
