package runtime

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var nginxVersionRe = regexp.MustCompile(`nginx/(\d+\.\d+\.\d+)`)

func detectNginx(ctx context.Context, dir, exe string) (Installed, error) {
	// nginx -v escreve em STDERR; run() combina os dois fluxos.
	out, err := run(ctx, dir, exe, "-v")
	if err != nil {
		return Installed{}, err
	}
	version, err := parseNginxVersion(out)
	if err != nil {
		return Installed{}, err
	}
	// nginx.org distribui só build 32-bit para Windows e não informa compilador; não inventar.
	return Installed{Kind: Nginx, Version: version, Major: version, Dir: dir, Exe: exe}, nil
}

// parseNginxVersion interpreta "nginx version: nginx/1.22.0".
func parseNginxVersion(out string) (string, error) {
	m := nginxVersionRe.FindStringSubmatch(out)
	if m == nil {
		return "", fmt.Errorf("runtime: saída inesperada do nginx: %q", strings.TrimSpace(out))
	}
	return m[1], nil
}
