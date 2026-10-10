package render

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hyphp/internal/runtime"
)

// fakePHP monta um runtime.Installed com um ExtDir povoado, que é o que
// RenderPHPIni consulta para decidir quais `extension=` emitir. Os módulos
// têm o nome de runtime.ExtFile, o mesmo que o render procura: php_<n>.dll no
// Windows, <n>.so no macOS. A pasta é <dir>/ext, a do Windows: o golden
// continua com "__PHPDIR__/ext".
func fakePHP(t *testing.T, major, version string, exts []string) runtime.Installed {
	t.Helper()
	dir := t.TempDir()
	extDir := filepath.Join(dir, "ext")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, e := range exts {
		if err := os.WriteFile(filepath.Join(extDir, runtime.ExtFile(e)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return runtime.Installed{
		Kind: runtime.PHP, Version: version, Major: major, Dir: dir,
		Exe: filepath.Join(dir, "php.exe"), CGIExe: filepath.Join(dir, "php-cgi.exe"),
		ExtDir: extDir,
	}
}

// shippedIn81 é o ext/ real do PHP 8.1.10 para Windows no que toca a
// DefaultExtensions: `zip` é estático e NÃO tem DLL.
var shippedIn81 = []string{
	"curl", "exif", "fileinfo", "gd", "intl", "mbstring", "opcache",
	"openssl", "pdo_mysql", "pdo_sqlite", "soap", "sodium", "sqlite3",
}

func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler golden %s: %v", path, err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), got) {
		os.WriteFile(path+".got", got, 0o644)
		t.Fatalf("saída difere de %s; gravei %s.got para diff", path, path)
	}
}

func TestRenderPHPIniDeterministico(t *testing.T) {
	inst := fakePHP(t, "8.1", "8.1.10", shippedIn81)
	a := RenderPHPIni(inst, []string{"curl", "gd", "intl"}, "C:/tmp", "C:/log", 1025, "", "", "", nil)
	b := RenderPHPIni(inst, []string{"curl", "gd", "intl"}, "C:/tmp", "C:/log", 1025, "", "", "", nil)
	if !bytes.Equal(a, b) {
		t.Fatal("duas chamadas iguais produziram bytes diferentes")
	}
	// Ordem de entrada e duplicatas não podem mudar a saída.
	c := RenderPHPIni(inst, []string{"intl", "curl", "gd", "curl"}, "C:/tmp", "C:/log", 1025, "", "", "", nil)
	if !bytes.Equal(a, c) {
		t.Fatalf("saída depende da ordem de entrada:\n%s\n---\n%s", a, c)
	}
	if !strings.Contains(string(a), "extension=curl\nextension=gd\nextension=intl\n") {
		t.Fatalf("extensões não saíram ordenadas:\n%s", a)
	}
}

func TestRenderPHPIniIgnoraExtensaoSemDLL(t *testing.T) {
	inst := fakePHP(t, "8.1", "8.1.10", shippedIn81)
	out := string(RenderPHPIni(inst, runtime.DefaultExtensions, "C:/tmp", "C:/log", 1025, "", "", "", nil))
	// Comparação por início de linha: no macOS a saída tem
	// "zend_extension=opcache.so", que contém "extension=opcache" como
	// substring e daria falso positivo.
	lines := "\n" + out
	if strings.Contains(lines, "\nextension=zip") {
		t.Fatalf("emitiu extension=zip sem %s; isso vira warning no corpo da resposta", runtime.ExtFile("zip"))
	}
	if !strings.Contains(out, "zend_extension="+opcacheFile()) {
		t.Fatal("opcache existe em ext/ e devia sair como zend_extension")
	}
	if strings.Contains(lines, "\nextension=opcache") {
		t.Fatal("opcache é zend_extension, nunca extension")
	}
}

// O bloco do usuário vem por último (no php.ini a última ocorrência vence),
// ordenado por nome, e não sai quando não há diretivas.
func TestRenderPHPIniDiretivasDoUsuario(t *testing.T) {
	inst := fakePHP(t, "8.1", "8.1.10", shippedIn81)
	user := map[string]string{"memory_limit": "1G", "max_input_vars": "5000", "date.timezone": "America/Sao_Paulo"}
	got := string(RenderPHPIni(inst, nil, "C:/tmp", "C:/log", 1025, "", "", "", user))
	wantTail := "opcache.revalidate_freq = 0\n" +
		"\n; Diretivas definidas pelo usuário em Runtimes › PHP (state.json, phpIni).\n" +
		"; Ficam no fim porque no php.ini a última ocorrência vence.\n" +
		"[PHP]\n" +
		"date.timezone = America/Sao_Paulo\n" +
		"max_input_vars = 5000\n" +
		"memory_limit = 1G\n"
	if !strings.HasSuffix(got, wantTail) {
		t.Fatalf("bloco do usuário errado; fim do arquivo:\n%s", got[max(0, len(got)-400):])
	}
	// O padrão continua acima; vale o do usuário por vir depois.
	if strings.Index(got, "memory_limit = 512M") > strings.Index(got, "memory_limit = 1G") {
		t.Fatal("override do usuário saiu antes do padrão do HyPHP")
	}
	for range 20 {
		if again := string(RenderPHPIni(inst, nil, "C:/tmp", "C:/log", 1025, "", "", "", user)); again != got {
			t.Fatal("ordem do bloco do usuário varia entre chamadas (iteração de map)")
		}
	}
	if a, b := RenderPHPIni(inst, nil, "C:/tmp", "C:/log", 1025, "", "", "", nil), RenderPHPIni(inst, nil, "C:/tmp", "C:/log", 1025, "", "", "", map[string]string{}); !bytes.Equal(a, b) || strings.Contains(string(a), "Runtimes › PHP") {
		t.Fatal("sem diretivas do usuário o arquivo não pode ganhar bloco")
	}
}

// Um state.json editado à mão não pode trocar o que o stack controla nem
// injetar linhas.
func TestRenderPHPIniIgnoraGerenciadasDoUsuario(t *testing.T) {
	inst := fakePHP(t, "8.1", "8.1.10", shippedIn81)
	got := string(RenderPHPIni(inst, nil, "C:/tmp", "C:/log", 1025, "", "", "", map[string]string{
		"extension_dir":    "C:/outro",
		"cgi.fix_pathinfo": "0",
		"max_input_vars":   "5000\nextension=evil",
		"sendmail_path":    "/usr/sbin/sendmail -t -i",
	}))
	if strings.Contains(got, "C:/outro") || strings.Contains(got, "cgi.fix_pathinfo = 0") || strings.Contains(got, "evil") || strings.Contains(got, "sendmail_path") {
		t.Fatalf("diretiva gerenciada ou multilinha foi escrita:\n%s", got)
	}
}

// Nome com "=" escapa de IsManagedIniDirective e o PHP leria
// "extension_dir = C:/x" da linha gerada.
func TestRenderPHPIniRecusaNomeInvalidoDoUsuario(t *testing.T) {
	inst := fakePHP(t, "8.1", "8.1.10", shippedIn81)
	got := string(RenderPHPIni(inst, nil, "C:/tmp", "C:/log", 1025, "", "", "", map[string]string{
		"extension_dir = C:/x ;": "1",
		"memory_limit":           "1G",
	}))
	if strings.Contains(got, "C:/x") {
		t.Fatalf("nome inválido foi escrito:\n%s", got)
	}
	if !strings.Contains(got, "\nmemory_limit = 1G\n") {
		t.Fatalf("nome válido devia continuar sendo escrito:\n%s", got)
	}
}

// Os sockets do MySQL só são gerenciados no macOS (phpini_darwin_test.go e
// ini_windows_test.go); aqui fica o que vale nos dois SOs.
func TestIsManagedIniDirective(t *testing.T) {
	for _, n := range []string{"extension_dir", "extension", "zend_extension", "error_log", "sys_temp_dir", "upload_tmp_dir", "session.save_path", "SMTP", "smtp", "smtp_port", "sendmail_from", "sendmail_path", "mail.add_x_header", "cgi.fix_pathinfo", "cgi.force_redirect", "cgi.rfc2616_headers", "fastcgi.impersonate", "user_ini.filename"} {
		if !IsManagedIniDirective(n) {
			t.Errorf("%s devia ser gerenciada", n)
		}
	}
	for _, n := range []string{"memory_limit", "max_input_vars", "session.gc_maxlifetime", "opcache.enable", "user_ini.cache_ttl", "extensions"} {
		if IsManagedIniDirective(n) {
			t.Errorf("%s não devia ser gerenciada", n)
		}
	}
}

// Todo padrão exposto à UI tem que sair no arquivo com o mesmo valor, e nenhum
// pode ser gerenciado (senão a UI ofereceria mudar algo que o serviço recusa).
func TestPHPIniDefaultsBatemComArquivo(t *testing.T) {
	inst := fakePHP(t, "8.1", "8.1.10", shippedIn81)
	got := string(RenderPHPIni(inst, nil, "C:/tmp", "C:/log", 1025, "", "", "", nil))
	for _, d := range PHPIniDefaults() {
		if !strings.Contains(got, "\n"+d.Name+" = "+d.Value+"\n") {
			t.Errorf("%s = %s não está no php.ini", d.Name, d.Value)
		}
		if IsManagedIniDirective(d.Name) {
			t.Errorf("%s é padrão editável e gerenciada ao mesmo tempo", d.Name)
		}
	}
}

// sendmail e o socket do MySQL só saem quando o stack os passa (macOS); com
// "" o arquivo do Windows fica como antes, sem linha nenhuma deles.
func TestPHPIniSendmailEMySQLSocket(t *testing.T) {
	inst := fakePHP(t, "8.3", "8.3.20", nil)
	sendmail := `"/opt/homebrew/opt/mailpit/bin/mailpit" sendmail -S 127.0.0.1:1025`
	sock := "/Users/dev/Library/Application Support/HyPHP/var/run/mysql.sock"
	got := string(RenderPHPIni(inst, nil, "/tmp", "/log", 1025, sendmail, sock, "", nil))
	for _, want := range []string{
		"\nsendmail_path = \"\\\"/opt/homebrew/opt/mailpit/bin/mailpit\\\" sendmail -S 127.0.0.1:1025\"\n",
		"\nmysqli.default_socket = \"" + sock + "\"\n",
		"\npdo_mysql.default_socket = \"" + sock + "\"\n",
		"\nmysql.default_socket = \"" + sock + "\"\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("faltou %q em:\n%s", want, got)
		}
	}

	vazio := string(RenderPHPIni(inst, nil, "/tmp", "/log", 1025, "", "", "", nil))
	for _, nao := range []string{"sendmail_path", "default_socket"} {
		if strings.Contains(vazio, nao) {
			t.Errorf("%s saiu sem valor:\n%s", nao, vazio)
		}
	}
}

// Sem ::1 na máquina o mysqld aborta ao não conseguir escutar num dos
// endereços da lista; aí só o IPv4 pode ir para o bind-address.
func TestRenderMyIniSemIPv6(t *testing.T) {
	got := string(RenderMyIni(3306, "C:/m", "C:/d", "C:/l", "", false, false))
	if !strings.Contains(got, "\nbind-address = 127.0.0.1\n") {
		t.Fatalf("bind-address devia ser só 127.0.0.1:\n%s", got)
	}
}

// Sem bundle, nenhuma linha de CA: apontar o PHP para um arquivo que não
// existe quebraria o HTTPS de saída que hoje funciona no macOS, com os
// certificados do Homebrew.
func TestSemBundleSemDiretivasDeCA(t *testing.T) {
	inst := fakePHP(t, "8.1", "8.1.10", shippedIn81)
	got := string(RenderPHPIni(inst, nil, "C:/tmp", "C:/log", 1025, "", "", "", nil))
	for _, nao := range []string{"curl.cainfo", "openssl.cafile"} {
		if strings.Contains(got, nao) {
			t.Errorf("%s saiu sem bundle:\n%s", nao, got)
		}
	}
}
