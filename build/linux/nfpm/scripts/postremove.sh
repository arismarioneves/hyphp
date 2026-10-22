#!/bin/sh
set -e

# O purge apaga a cópia do .deb que o install-update deixa para o apt-get.
if [ "$1" = "purge" ]; then
    rm -rf /var/cache/hyphp
fi

exit 0
