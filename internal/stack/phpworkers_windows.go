package stack

import (
	"fmt"
	"path/filepath"

	"hyphp/internal/runtime"
	"hyphp/internal/supervisor"
)

// phpPoolPorts: no Windows cada worker php-cgi atende um request por vez e
// precisa da própria porta, então a série reserva PoolSize portas.
func phpPoolPorts(poolSize int) int { return poolSize }

// phpSeriesSpecs devolve um worker php-cgi por porta, IDs php:<série>:<i>.
func phpSeriesSpecs(inst runtime.Installed, major string, ports []int, etcDir, logDir string) []supervisor.Spec {
	specs := make([]supervisor.Spec, 0, len(ports))
	for i, port := range ports {
		specs = append(specs, phpWorkerSpec(inst, major, i, port, etcDir, logDir))
	}
	return specs
}

// phpWorkerSpec é a receita C16: php-cgi em modo FastCGI externo, sem limite
// de requests por processo (o default 500 mataria o worker sob carga).
func phpWorkerSpec(inst runtime.Installed, major string, i, port int, etcDir, logDir string) supervisor.Spec {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	return supervisor.Spec{
		ID:    fmt.Sprintf("php:%s:%d", major, i),
		Name:  fmt.Sprintf("PHP %s #%d", major, i),
		Group: "php",
		Exe:   inst.CGIExe,
		Args: []string{
			"-b", addr,
			"-c", filepath.Join(etcDir, "php", major, "php.ini"),
			"-d", "cgi.force_redirect=0",
			"-d", "cgi.fix_pathinfo=1",
		},
		Env:     []string{"PHP_FCGI_MAX_REQUESTS=0"},
		Dir:     inst.Dir,
		Port:    port,
		Probe:   &supervisor.TCPProbe{Addr: addr},
		Restart: defaultRestart(),
		LogPath: filepath.Join(logDir, fmt.Sprintf("php-%s-%d.log", major, i)),
	}
}
