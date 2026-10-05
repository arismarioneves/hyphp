package stack

import (
	"net"
	"path/filepath"
	"strconv"

	"hyphp/internal/runtime"
	"hyphp/internal/supervisor"
)

// phpPoolPorts: no Mac a série é um master php-fpm com um pool numa porta só;
// os filhos (pm.max_children = PoolSize no php-fpm.conf) dividem o socket.
func phpPoolPorts(int) int { return 1 }

// phpSeriesSpecs devolve o master php-fpm da série. O ID fica php:<série>:0
// para que o reinício por mudança de php.ini (applySpecs), os pools da UI e
// specsUsingDir sigam funcionando sem saber de SO.
//
// --nodaemonize: o supervisor acompanha o processo. --force-stderr: sem TTY o
// fpm escreveria o log no error_log e não no stderr que o supervisor captura.
// --fpm-config deixa de fora o php-fpm.conf do Homebrew (e o pool www na
// :9000). PHP_INI_SCAN_DIR vazio desliga o conf.d do Homebrew, que carregaria
// o OPcache de novo e vazaria .ini de pecl.
func phpSeriesSpecs(inst runtime.Installed, major string, ports []int, etcDir, logDir string) []supervisor.Spec {
	port := ports[0]
	dir := filepath.Join(etcDir, "php", major)
	return []supervisor.Spec{{
		ID:    "php:" + major + ":0",
		Name:  "PHP " + major,
		Group: "php",
		Exe:   inst.CGIExe,
		Args: []string{
			"--nodaemonize", "--force-stderr",
			"--fpm-config", filepath.Join(dir, "php-fpm.conf"),
			"-c", filepath.Join(dir, "php.ini"),
		},
		Env:     []string{"PHP_INI_SCAN_DIR="},
		Dir:     inst.Dir,
		Port:    port,
		Probe:   &supervisor.TCPProbe{Addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))},
		Restart: defaultRestart(),
		LogPath: filepath.Join(logDir, "php-"+major+"-0.log"),
	}}
}
