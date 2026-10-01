package render

import (
	"bytes"
	"path/filepath"
	"testing"

	"hyphp/internal/runtime"
)

// O golden foi gerado no Windows: traz php_opcache.dll e o
// fastcgi.impersonate do php-cgi, que o php.ini do macOS não tem.
func TestRenderPHPIniGolden(t *testing.T) {
	inst := fakePHP(t, "8.1", "8.1.10", shippedIn81)
	got := RenderPHPIni(inst, runtime.DefaultExtensions, "C:/hyphp/var/tmp/php/8.1", "C:/hyphp/log", 1025, nil)
	got = bytes.ReplaceAll(got, []byte(filepath.ToSlash(inst.Dir)), []byte("__PHPDIR__"))
	checkGolden(t, "php-8.1.ini.golden", got)
}
