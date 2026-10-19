package main

import "path/filepath"

// cliExe é a CLI em <pasta do app>/cli/hyphp, que no build local é
// bin/cli/hyphp.
func cliExe(appExe string) string { return filepath.Join(filepath.Dir(appExe), "cli", "hyphp") }
