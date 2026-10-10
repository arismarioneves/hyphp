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

// IniDirective é uma diretiva de php.ini com o valor que o HyPHP escreve.
type IniDirective struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Os padrões do HyPHP ficam em grupos porque o php.ini os escreve em blocos
// separados, intercalados com diretivas gerenciadas (error_log, sys_temp_dir):
// manter a ordem de antes deixa o arquivo gerado idêntico byte a byte.
var (
	errorDefaults = []IniDirective{
		{"error_reporting", "E_ALL"},
		{"display_errors", "On"},
		{"display_startup_errors", "On"},
		{"log_errors", "On"},
	}
	limitDefaults = []IniDirective{
		{"upload_max_filesize", "64M"},
		{"post_max_size", "64M"},
		{"memory_limit", "512M"},
		{"max_execution_time", "120"},
		{"date.timezone", "UTC"},
	}
	opcacheDefaults = []IniDirective{
		{"opcache.enable", "1"},
		{"opcache.enable_cli", "0"},
		{"opcache.validate_timestamps", "1"},
		{"opcache.revalidate_freq", "0"},
	}
)

// PHPIniDefaults devolve, na ordem do arquivo, as diretivas que o HyPHP define
// e que o usuário pode sobrescrever por série. A UI mostra estes valores como
// "padrão do HyPHP".
func PHPIniDefaults() []IniDirective {
	return slices.Concat(errorDefaults, limitDefaults, opcacheDefaults)
}

// managedDirectives são as diretivas que o HyPHP controla nos dois SOs:
// caminhos que o stack cria por série (ext, log, tmp), o redirecionamento de
// mail() para o Mailpit e o que o php-cgi/php-fpm precisa para atender o
// Apache/nginx. Trocar qualquer uma quebra o stack de um jeito que o usuário
// não liga ao php.ini. sendmail_path só sai no macOS, mas fica gerenciada nos
// dois: no Windows não tem efeito no stack e bloqueá-la não tira nada de quem
// usa SMTP. O que só um SO controla fica em osManagedDirectives.
var managedDirectives = []string{
	"extension_dir", "extension", "zend_extension", "error_log",
	"sys_temp_dir", "upload_tmp_dir", "session.save_path",
	"smtp", "smtp_port", "sendmail_from", "sendmail_path", "mail.add_x_header",
	"fastcgi.impersonate", "user_ini.filename",
}

// IsManagedIniDirective diz se name é gerenciada pelo HyPHP e portanto não pode
// ser sobrescrita pelo usuário. Compara sem caixa porque o php.ini escreve
// "SMTP" em maiúsculas; `cgi.*` inteiro é do php-cgi (fix_pathinfo errado abre
// execução de arquivo arbitrário via PATH_INFO).
func IsManagedIniDirective(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(n, "cgi.") || slices.Contains(managedDirectives, n) || slices.Contains(osManagedDirectives, n)
}

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
//
// userIni são as diretivas que o usuário definiu para a série (state.PHPIni).
// Saem num bloco no FIM do arquivo porque no php.ini a última ocorrência
// vence: assim sobrescrevem os padrões acima sem que o renderer precise
// removê-los. O chamador já recusou as gerenciadas (IsManagedIniDirective).
//
// sendmail e mysqlSocket vazios não emitem linha: o Windows passa "" e o
// arquivo sai igual ao de antes. No macOS o SMTP/smtp_port acima não vale
// (mail() usa sendmail_path), e o PHP do Homebrew tem /tmp/mysql.sock
// compilado como socket padrão, que não é o do mysqld do HyPHP.
//
// caFile é o bundle de CAs (etc/ssl/cacert.pem); vazio não emite linha, como
// no macOS e no Windows antes do primeiro download.
func RenderPHPIni(inst runtime.Installed, enabledExt []string, tmpDir, logDir string, smtpPort int, sendmail, mysqlSocket, caFile string, userIni map[string]string) []byte {
	extDir := filepath.ToSlash(inst.ExtDir)
	tmp := SlashDir(tmpDir)
	log := SlashDir(logDir)

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
		fmt.Fprintf(&b, "zend_extension=%s\n", opcacheFile())
	}

	fmt.Fprintf(&b, "\n")
	writeDirectives(&b, errorDefaults)
	fmt.Fprintf(&b, "error_log = %q\n", log+"/php-"+inst.Major+".log")

	fmt.Fprintf(&b, "\n")
	writeDirectives(&b, limitDefaults)

	fmt.Fprintf(&b, "\nsys_temp_dir = %q\n", tmp)
	fmt.Fprintf(&b, "upload_tmp_dir = %q\n", tmp)
	fmt.Fprintf(&b, "session.save_path = %q\n", tmp)

	fmt.Fprintf(&b, "\n[CGI]\n")
	fmt.Fprintf(&b, "cgi.force_redirect = 0\n")
	fmt.Fprintf(&b, "cgi.fix_pathinfo = 1\n")
	if cgiImpersonate {
		fmt.Fprintf(&b, "fastcgi.impersonate = 0\n")
	}

	// Redireciona mail() para o Mailpit. É o ponto central da Fase 8: em
	// desenvolvimento nenhum e-mail pode sair para o mundo — um teste de
	// "recuperar senha" não deve alcançar o endereço real do cliente.
	// sendmail_from é obrigatório no Windows: sem ele o PHP aborta com
	// "Bad Message Return Path" antes mesmo de abrir a conexão SMTP.
	fmt.Fprintf(&b, "\n[mail function]\n")
	fmt.Fprintf(&b, "SMTP = 127.0.0.1\n")
	fmt.Fprintf(&b, "smtp_port = %d\n", smtpPort)
	fmt.Fprintf(&b, "sendmail_from = hyphp@localhost\n")
	fmt.Fprintf(&b, "mail.add_x_header = On\n")
	// %q escapa as aspas internas como \", que o php.ini desfaz: o valor
	// chega ao /bin/sh com o caminho do Mailpit entre aspas.
	if sendmail != "" {
		fmt.Fprintf(&b, "sendmail_path = %q\n", sendmail)
	}

	// host=localhost no mysqli/PDO conecta pelo socket Unix; sem estas linhas
	// iria ao /tmp/mysql.sock, que pode ser de um MySQL do `brew services`.
	if mysqlSocket != "" {
		sock := filepath.ToSlash(mysqlSocket)
		fmt.Fprintf(&b, "\n[MySQL]\n")
		fmt.Fprintf(&b, "mysqli.default_socket = %q\n", sock)
		fmt.Fprintf(&b, "pdo_mysql.default_socket = %q\n", sock)
		fmt.Fprintf(&b, "mysql.default_socket = %q\n", sock)
	}

	// O PHP do Windows não traz certificados de CA: sem cainfo e cafile, todo
	// HTTPS de saída (curl, file_get_contents, SoapClient) falha com
	// "curl error 60".
	if caFile != "" {
		ca := filepath.ToSlash(caFile)
		fmt.Fprintf(&b, "\n[curl]\n")
		fmt.Fprintf(&b, "curl.cainfo = %q\n", ca)
		fmt.Fprintf(&b, "\n[openssl]\n")
		fmt.Fprintf(&b, "openssl.cafile = %q\n", ca)
	}

	fmt.Fprintf(&b, "\n[opcache]\n")
	writeDirectives(&b, opcacheDefaults)

	writeUserIni(&b, userIni)
	return b.Bytes()
}

func writeDirectives(b *bytes.Buffer, list []IniDirective) {
	for _, d := range list {
		fmt.Fprintf(b, "%s = %s\n", d.Name, d.Value)
	}
}

// writeUserIni escreve o bloco do usuário ordenado por nome (determinismo).
// Pula nomes fora de runtime.ValidIniName, gerenciadas e quebras de linha
// mesmo já recusadas pelo serviço: o state.json pode ter sido editado à mão, e
// um nome com "=" ou uma linha a mais aqui viraria uma diretiva que ninguém
// pediu.
func writeUserIni(b *bytes.Buffer, userIni map[string]string) {
	names := make([]string, 0, len(userIni))
	for name, value := range userIni {
		if !runtime.ValidIniName(name) || IsManagedIniDirective(name) || strings.ContainsAny(value, "\r\n") {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return
	}
	sort.Strings(names)
	fmt.Fprintf(b, "\n; Diretivas definidas pelo usuário em Runtimes › PHP (state.json, phpIni).\n")
	fmt.Fprintf(b, "; Ficam no fim porque no php.ini a última ocorrência vence.\n")
	fmt.Fprintf(b, "[PHP]\n")
	for _, name := range names {
		fmt.Fprintf(b, "%s = %s\n", name, userIni[name])
	}
}

// hasExtensionDLL responde se o módulo de name (runtime.ExtFile) existe em extDir.
func hasExtensionDLL(extDir, name string) bool {
	st, err := os.Stat(filepath.Join(filepath.FromSlash(extDir), runtime.ExtFile(name)))
	return err == nil && !st.IsDir()
}

// opcacheFile é o arquivo do opcache, carregado como zend_extension pelo nome
// do arquivo e não pelo nome da extensão.
func opcacheFile() string { return runtime.ExtFile("opcache") }

// SlashDir normaliza um diretório para "/" sem barra final. Apache e nginx
// aceitam "/" no Windows e a barra final duplicaria separadores nos Include.
func SlashDir(dir string) string {
	s := filepath.ToSlash(dir)
	if len(s) > 1 {
		s = strings.TrimSuffix(s, "/")
	}
	return s
}
