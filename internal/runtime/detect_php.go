package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// phpProbe imprime "VERSÃO|PHP_INT_SIZE|zts|nts" — três fatos num único exec.
// Exemplo real (php-8.1.10-Win32-vs16-x64): "8.1.10|8|zts".
const phpProbe = "echo PHP_VERSION.'|'.PHP_INT_SIZE.'|'.(ZEND_THREAD_SAFE?'zts':'nts');"

func detectPHP(ctx context.Context, dir, exe string) (Installed, error) {
	// -n ignora qualquer php.ini na pasta: um ini com extensão quebrada poluiria stdout com warnings.
	out, err := run(ctx, dir, exe, "-n", "-r", phpProbe)
	if err != nil {
		return Installed{}, err
	}
	version, intSize, zts, err := parsePHPProbe(out)
	if err != nil {
		return Installed{}, err
	}
	compiler, arch := parseBuildTags(filepath.Base(dir))
	if arch == "" {
		if intSize == 8 {
			arch = "x64"
		} else {
			arch = "x86"
		}
	}
	return Installed{
		Kind:       PHP,
		Version:    version,
		Major:      MajorOf(version),
		Dir:        dir,
		Exe:        exe,
		CGIExe:     filepath.Join(dir, "php-cgi.exe"),
		Compiler:   compiler,
		Arch:       arch,
		ThreadSafe: &zts,
	}, nil
}

// parsePHPProbe interpreta a saída de phpProbe. Rejeita qualquer texto antes da versão.
func parsePHPProbe(out string) (version string, intSize int, zts bool, err error) {
	parts := strings.Split(strings.TrimSpace(out), "|")
	if len(parts) != 3 || !strictVersionRe.MatchString(parts[0]) {
		return "", 0, false, fmt.Errorf("runtime: saída inesperada do php: %q", strings.TrimSpace(out))
	}
	intSize, err = strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, false, fmt.Errorf("runtime: PHP_INT_SIZE inválido em %q", strings.TrimSpace(out))
	}
	switch parts[2] {
	case "zts":
		zts = true
	case "nts":
		zts = false
	default:
		return "", 0, false, fmt.Errorf("runtime: thread-safety inválida em %q", strings.TrimSpace(out))
	}
	return parts[0], intSize, zts, nil
}
