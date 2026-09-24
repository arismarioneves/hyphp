package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var apacheVersionRe = regexp.MustCompile(`Apache/(\d+\.\d+\.\d+)`)

func detectApache(ctx context.Context, dir, exe string) (Installed, error) {
	out, err := run(ctx, dir, exe, "-v")
	if err != nil {
		return Installed{}, err
	}
	version, compiler, arch, err := parseApacheVersion(out)
	if err != nil {
		return Installed{}, err
	}
	// Builds sem "Apache Lounge VSxx" no banner: cai para o nome da pasta (httpd-2.4.68-260920-Win64-VS18).
	if compiler == "" || arch == "" {
		c, a := parseBuildTags(filepath.Base(dir))
		if compiler == "" {
			compiler = c
		}
		if arch == "" {
			arch = a
		}
	}
	return Installed{Kind: Apache, Version: version, Major: version, Dir: dir, Exe: exe, Compiler: compiler, Arch: arch}, nil
}

// parseApacheVersion interpreta `httpd.exe -v`:
//
//	Server version: Apache/2.4.54 (Win64)
//	Apache Lounge VS16 Server built:   Jun 22 2022 09:58:15
func parseApacheVersion(out string) (version, compiler, arch string, err error) {
	m := apacheVersionRe.FindStringSubmatch(out)
	if m == nil {
		return "", "", "", fmt.Errorf("runtime: saída inesperada do httpd: %q", strings.TrimSpace(out))
	}
	compiler, arch = parseBuildTags(out)
	return m[1], compiler, arch, nil
}
