package render

import (
	"strings"
	"testing"
)

// blowfish_secret precisa ter exatamente 32 bytes: o phpMyAdmin recusa a
// sessão e mostra um aviso permanente com qualquer outro tamanho.
func TestBlowfishSecretTem32Bytes(t *testing.T) {
	s, err := NewBlowfishSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 32 {
		t.Errorf("len = %d, quero 32", len(s))
	}

	outro, err := NewBlowfishSecret()
	if err != nil {
		t.Fatal(err)
	}
	if s == outro {
		t.Error("dois segredos iguais: a origem não é aleatória")
	}
}

// A config precisa apontar para a porta configurada e aceitar root sem senha,
// que é como o HyPHP inicializa o MySQL. TempDir gravável evita o aviso
// permanente de cache de templates que o phpMyAdmin exibe sem ele.
func TestPhpMyAdminConfig(t *testing.T) {
	got := string(PhpMyAdminConfig(3307, "12345678901234567890123456789012", `C:\hyphp\var\tmp\phpmyadmin`))
	for _, want := range []string{
		`$cfg['Servers'][$i]['host'] = '127.0.0.1';`,
		`$cfg['Servers'][$i]['port'] = '3307';`,
		`$cfg['Servers'][$i]['AllowNoPassword'] = true;`,
		`$cfg['blowfish_secret'] = '12345678901234567890123456789012';`,
		`$cfg['TempDir'] = 'C:/hyphp/var/tmp/phpmyadmin';`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("falta %q em:\n%s", want, got)
		}
	}
}

// Sem auth_type 'config' com usuário preenchido o phpMyAdmin abre um formulário
// de login — e o HyPHP não tem credencial para oferecer ao usuário digitar.
func TestPhpMyAdminConfigEntraSemLogin(t *testing.T) {
	got := string(PhpMyAdminConfig(3306, "12345678901234567890123456789012", "C:/tmp/pma"))
	for _, want := range []string{
		`$cfg['Servers'][$i]['auth_type'] = 'config';`,
		`$cfg['Servers'][$i]['user'] = 'root';`,
		`$cfg['Servers'][$i]['password'] = '';`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("falta %q em:\n%s", want, got)
		}
	}
}

// O segredo e o caminho entram em string PHP de aspas simples; uma aspa ou
// contrabarra crua fecharia a string e o config.inc.php viraria um parse error
// que derruba a ferramenta inteira.
func TestPhpMyAdminConfigEscapaAspas(t *testing.T) {
	got := string(PhpMyAdminConfig(3306, `a'b\c`, `C:\pasta'do usuário`))
	if !strings.Contains(got, `$cfg['blowfish_secret'] = 'a\'b\\c';`) {
		t.Errorf("segredo não escapado:\n%s", got)
	}
	if !strings.Contains(got, `$cfg['TempDir'] = 'C:/pasta\'do usuário';`) {
		t.Errorf("TempDir não escapado:\n%s", got)
	}
}
