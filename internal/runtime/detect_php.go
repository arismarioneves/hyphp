package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// phpProbe imprime "VERSÃO|PHP_INT_SIZE|zts|nts|basename(PHP_EXTENSION_DIR)"
// — quatro fatos num único exec. Exemplos reais: php-8.1.10-Win32-vs16-x64
// dá "8.1.10|8|zts|ext"; o php@8.3 do Homebrew dá "8.3.20|8|nts|20230831".
// Só o basename da pasta de módulos interessa (ver phpExtDir).
const phpProbe = "echo PHP_VERSION.'|'.PHP_INT_SIZE.'|'.(ZEND_THREAD_SAFE?'zts':'nts').'|'.basename(PHP_EXTENSION_DIR);"

func detectPHP(ctx context.Context, dir, exe string) (Installed, error) {
	// -n ignora qualquer php.ini na pasta: um ini com extensão quebrada poluiria stdout com warnings.
	out, err := run(ctx, dir, exe, "-n", "-r", phpProbe)
	if err != nil {
		return Installed{}, err
	}
	version, intSize, zts, apiBase, err := parsePHPProbe(out)
	if err != nil {
		return Installed{}, err
	}
	compiler, arch := parseBuildTags(filepath.Base(dir))
	if arch == "" {
		arch = phpArch(intSize)
	}
	return Installed{
		Kind:       PHP,
		Version:    version,
		Major:      MajorOf(version),
		Dir:        dir,
		Exe:        exe,
		CGIExe:     filepath.Join(dir, workerFile()),
		Compiler:   compiler,
		Arch:       arch,
		ThreadSafe: &zts,
		ExtDir:     phpExtDir(dir, apiBase),
	}, nil
}

// parsePHPProbe interpreta a saída de phpProbe. Rejeita qualquer texto antes da
// versão e a pasta de módulos vazia, que no macOS viraria <keg>/lib/php.
func parsePHPProbe(out string) (version string, intSize int, zts bool, apiBase string, err error) {
	trimmed := strings.TrimSpace(out)
	parts := strings.Split(trimmed, "|")
	if len(parts) != 4 || !strictVersionRe.MatchString(parts[0]) || parts[3] == "" {
		return "", 0, false, "", fmt.Errorf("runtime: saída inesperada do php: %q", trimmed)
	}
	intSize, err = strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, false, "", fmt.Errorf("runtime: PHP_INT_SIZE inválido em %q", trimmed)
	}
	switch parts[2] {
	case "zts":
		zts = true
	case "nts":
		zts = false
	default:
		return "", 0, false, "", fmt.Errorf("runtime: thread-safety inválida em %q", trimmed)
	}
	return parts[0], intSize, zts, parts[3], nil
}
