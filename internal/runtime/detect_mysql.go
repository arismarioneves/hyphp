package runtime

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var mysqlVersionRe = regexp.MustCompile(`\bVer (\d+\.\d+\.\d+)`)

func detectMySQL(ctx context.Context, dir, exe string) (Installed, error) {
	// --no-defaults evita ler um my.ini que aponte para datadir inexistente.
	out, err := run(ctx, dir, exe, "--no-defaults", "--version")
	if err != nil {
		return Installed{}, err
	}
	version, arch, err := parseMySQLVersion(out)
	if err != nil {
		return Installed{}, err
	}
	return Installed{Kind: MySQL, Version: version, Major: version, Dir: dir, Exe: exe, Arch: arch}, nil
}

// parseMySQLVersion interpreta `mysqld.exe --version`:
//
//	C:\...\mysqld.exe  Ver 8.0.30 for Win64 on x86_64 (MySQL Community Server - GPL)
func parseMySQLVersion(out string) (version, arch string, err error) {
	m := mysqlVersionRe.FindStringSubmatch(out)
	if m == nil {
		return "", "", fmt.Errorf("runtime: saída inesperada do mysqld: %q", strings.TrimSpace(out))
	}
	// "for Win64" → x64. "x86_64" não casa com \bx86\b porque '_' é caractere de palavra.
	_, arch = parseBuildTags(out)
	return m[1], arch, nil
}
