package main

import "path/filepath"

// cliExe é a CLI que o pacote põe em <prefixo>/bin/hyphp, ao lado do app em
// <prefixo>/lib/hyphp/hyphp (/usr/bin e /usr/lib no .deb).
func cliExe(appExe string) string {
	return filepath.Clean(filepath.Join(filepath.Dir(appExe), "..", "..", "bin", "hyphp"))
}
