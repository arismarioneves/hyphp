package main

import "path/filepath"

// cliExe é a CLI que o instalador põe em <instalação>\cli\hyphp.exe; a aba
// CLI mostra esse caminho e o põe no PATH.
func cliExe(appExe string) string { return filepath.Join(filepath.Dir(appExe), "cli", "hyphp.exe") }
