package runtime

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// mariadbVersionRe exige o sufixo "-MariaDB": o mysqld.exe que o zip do
// MariaDB também traz responde no mesmo formato do MySQL, e a pasta tem de ser
// reconhecida como MariaDB, não como um MySQL com versão 10 ou 11.
var mariadbVersionRe = regexp.MustCompile(`\bVer (\d+\.\d+\.\d+)-MariaDB`)

func detectMariaDB(ctx context.Context, dir, exe string) (Installed, error) {
	// --no-defaults pelo mesmo motivo do MySQL: um my.ini com datadir que não
	// existe faria o --version falhar.
	out, err := run(ctx, dir, exe, "--no-defaults", "--version")
	if err != nil {
		return Installed{}, err
	}
	version, arch, err := parseMariaDBVersion(out)
	if err != nil {
		return Installed{}, err
	}
	return Installed{Kind: MariaDB, Version: version, Major: version, Dir: dir, Exe: exe, Arch: arch}, nil
}

// parseMariaDBVersion interpreta `mariadbd.exe --version`:
//
//	mariadbd.exe  Ver 11.4.13-MariaDB for Win64 on AMD64 (mariadb.org binary distribution)
func parseMariaDBVersion(out string) (version, arch string, err error) {
	m := mariadbVersionRe.FindStringSubmatch(out)
	if m == nil {
		return "", "", fmt.Errorf("runtime: saída inesperada do mariadbd: %q", strings.TrimSpace(out))
	}
	_, arch = parseBuildTags(out)
	return m[1], arch, nil
}
