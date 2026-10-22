#!/bin/sh
set -e

# O dpkg chama com configure na instalação e no upgrade; o perfil é recarregado
# nos dois para valer o caminho e as regras da versão nova.
if [ "$1" = "configure" ]; then
    # Sem AppArmor ativo o perfil não serve para nada. Um apparmor_parser sem
    # a ABI 4 recusa o perfil: o app ainda abre, então só avisa.
    if aa-enabled --quiet 2>/dev/null; then
        if ! apparmor_parser -r -T -W /etc/apparmor.d/hyphp; then
            echo "hyphp: could not load the AppArmor profile /etc/apparmor.d/hyphp" >&2
        fi
    fi
fi

exit 0
