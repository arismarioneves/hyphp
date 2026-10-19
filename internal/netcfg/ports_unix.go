//go:build darwin || linux

package netcfg

import (
	"errors"
	"syscall"
)

// bindOccupied separa porta ocupada de bind recusado por permissão. Sem root,
// o macOS só deixa escutar abaixo de 1024 no endereço curinga, que é como o
// Apache e o nginx do HyPHP escutam, e o Linux recusa em qualquer endereço
// abaixo do net.ipv4.ip_unprivileged_port_start. O bind de teste em
// 127.0.0.1:80 dá EACCES com a porta livre e virava o aviso falso "porta 80 em
// uso". Com o EACCES fora da conta, quem decide são os outros binds e a lista
// de listeners em seguida.
func bindOccupied(err error) bool { return !errors.Is(err, syscall.EACCES) }
