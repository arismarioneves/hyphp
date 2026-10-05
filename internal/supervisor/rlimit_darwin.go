package supervisor

import "syscall"

// RaiseFileLimit eleva o limite soft de arquivos abertos do processo.
//
// App de GUI lançado pelo launchd nasce com RLIMIT_NOFILE soft = 256. O Go
// (desde 1.21) sobe o próprio soft para o hard no startup, mas guarda o valor
// original e o restaura em todo filho do os/exec — a menos que o programa
// chame Setrlimit(RLIMIT_NOFILE) ele mesmo, o que descarta o valor guardado.
// nginx/php-fpm/mysqld têm diretiva própria, mas o httpd não tem; uma chamada
// aqui no startup cobre todos os filhos de uma vez.
func RaiseFileLimit() error {
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		return err
	}
	// 65536 é teto confortável; o macOS rejeita valores acima de OPEN_MAX
	// mesmo com hard "unlimited", então não usamos o hard cru.
	want := uint64(65536)
	if lim.Max < want {
		want = lim.Max
	}
	if lim.Cur < want {
		lim.Cur = want
	}
	return syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lim)
}
