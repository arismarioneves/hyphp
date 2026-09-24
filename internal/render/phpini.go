package render

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"hyphp/internal/runtime"
)

// RenderPHPIni gera o php.ini de uma série de PHP. Determinístico: mesma
// entrada, mesmos bytes.
//
// Só emite `extension=` (e `zend_extension=`) para DLL que existe de fato em
// <inst.Dir>/ext. Motivo concreto: com display_errors=On — que é o ponto de um
// ambiente de desenvolvimento — o PHP imprime "Unable to load dynamic library"
// no CORPO da resposta CGI, sujando a primeira linha de toda página. E o caso
// não é hipotético: `zip` está em runtime.DefaultExtensions mas é compilado
// estaticamente no PHP 8.x para Windows, então php_zip.dll não existe.
//
// tmpDir e logDir são absolutos e já vêm com "/" (o chamador usa
// filepath.ToSlash). tmpDir é por série: sys_temp_dir, upload_tmp_dir e
// session.save_path apontam todos para lá, para que uma sessão criada sob 8.1
// não seja lida por um worker 7.2.
func RenderPHPIni(inst runtime.Installed, enabledExt []string, tmpDir, logDir string) []byte {
	extDir := filepath.ToSlash(filepath.Join(inst.Dir, "ext"))
	tmp := slashDir(tmpDir)
	log := slashDir(logDir)

	names := slices.Clone(enabledExt)
	sort.Strings(names)
	names = slices.Compact(names)

	var b bytes.Buffer
	fmt.Fprintf(&b, "; Gerado pelo HyPHP — não editar à mão.\n")
	fmt.Fprintf(&b, "; Fonte: internal/render/phpini.go — reescrito a cada Reconcile.\n")
	fmt.Fprintf(&b, "; PHP %s (série %s)\n\n", inst.Version, inst.Major)

	fmt.Fprintf(&b, "[PHP]\n")
	fmt.Fprintf(&b, "extension_dir = %q\n", extDir)
	for _, name := range names {
		if name == "" || name == "opcache" {
			continue // opcache é zend_extension, tratado abaixo
		}
		if hasExtensionDLL(extDir, name) {
			fmt.Fprintf(&b, "extension=%s\n", name)
		}
	}
	if hasExtensionDLL(extDir, "opcache") {
		fmt.Fprintf(&b, "zend_extension=php_opcache.dll\n")
	}

	fmt.Fprintf(&b, "\nerror_reporting = E_ALL\n")
	fmt.Fprintf(&b, "display_errors = On\n")
	fmt.Fprintf(&b, "display_startup_errors = On\n")
	fmt.Fprintf(&b, "log_errors = On\n")
	fmt.Fprintf(&b, "error_log = %q\n", log+"/php-"+inst.Major+".log")

	fmt.Fprintf(&b, "\nupload_max_filesize = 64M\n")
	fmt.Fprintf(&b, "post_max_size = 64M\n")
	fmt.Fprintf(&b, "memory_limit = 512M\n")
	fmt.Fprintf(&b, "max_execution_time = 120\n")
	fmt.Fprintf(&b, "date.timezone = UTC\n")

	fmt.Fprintf(&b, "\nsys_temp_dir = %q\n", tmp)
	fmt.Fprintf(&b, "upload_tmp_dir = %q\n", tmp)
	fmt.Fprintf(&b, "session.save_path = %q\n", tmp)

	fmt.Fprintf(&b, "\n[CGI]\n")
	fmt.Fprintf(&b, "cgi.force_redirect = 0\n")
	fmt.Fprintf(&b, "cgi.fix_pathinfo = 1\n")
	fmt.Fprintf(&b, "fastcgi.impersonate = 0\n")

	fmt.Fprintf(&b, "\n[opcache]\n")
	fmt.Fprintf(&b, "opcache.enable = 1\n")
	fmt.Fprintf(&b, "opcache.enable_cli = 0\n")
	fmt.Fprintf(&b, "opcache.validate_timestamps = 1\n")
	fmt.Fprintf(&b, "opcache.revalidate_freq = 0\n")

	return b.Bytes()
}

// hasExtensionDLL responde se <extDir>/php_<name>.dll existe.
func hasExtensionDLL(extDir, name string) bool {
	st, err := os.Stat(filepath.Join(filepath.FromSlash(extDir), "php_"+name+".dll"))
	return err == nil && !st.IsDir()
}

// slashDir normaliza um diretório para "/" sem barra final.
func slashDir(dir string) string {
	s := filepath.ToSlash(dir)
	if len(s) > 1 {
		s = strings.TrimSuffix(s, "/")
	}
	return s
}
