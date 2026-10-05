package main

import "path/filepath"

// cliExe é a CLI dentro do bundle: HyPHP.app/Contents/Helpers/hyphp.
// Contents/Helpers é um dos lugares que a Apple aceita para executável
// auxiliar assinado junto com o app; Contents/MacOS/hyphp já é o próprio app.
func cliExe(appExe string) string {
	return filepath.Clean(filepath.Join(filepath.Dir(appExe), "..", "Helpers", "hyphp"))
}
