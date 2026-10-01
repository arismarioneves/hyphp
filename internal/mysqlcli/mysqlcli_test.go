package mysqlcli

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"hyphp/internal/runtime"
	"hyphp/internal/sysproc"
)

func TestValidateName(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"simples", "acme_dev", false},
		{"só dígitos", "12345", false},
		{"maiúsculas", "AcmeDev", false},
		{"64 chars", strings.Repeat("a", 64), false},
		{"65 chars", strings.Repeat("a", 65), true},
		{"vazio", "", true},
		{"crase", "acme`", true},
		{"injeção com crase", "a` ; DROP DATABASE `b", true},
		{"espaço", "acme dev", true},
		{"ponto e vírgula", "acme;", true},
		{"hífen", "acme-dev", true},
		{"barra", "acme/dev", true},
		{"unicode", "açme", true},
		{"newline", "acme\ndrop", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := ValidateName(c.in); (err != nil) != c.wantErr {
				t.Fatalf("ValidateName(%q) err = %v, wantErr = %v", c.in, err, c.wantErr)
			}
		})
	}
}

func TestIsSystemSchema(t *testing.T) {
	for _, n := range []string{"mysql", "MySQL", "information_schema", "performance_schema", "sys"} {
		if !IsSystemSchema(n) {
			t.Fatalf("%q deveria ser schema de sistema", n)
		}
	}
	if IsSystemSchema("acme_dev") {
		t.Fatal("acme_dev não é schema de sistema")
	}
}

func TestParseDatabases(t *testing.T) {
	// Saída real de `mysql.exe -N -B -e <listQuery>`: TSV, CRLF, sem cabeçalho.
	out := "acme_dev\t0.00\r\n" +
		"information_schema\t0.16\r\n" +
		"loja_2024\t12.53\r\n" +
		"mysql\t2.44\r\n" +
		"performance_schema\t0.00\r\n" +
		"sem_tabelas\tNULL\r\n" +
		"sys\t0.02\r\n" +
		"\r\n"
	want := []DB{
		{Name: "acme_dev", SizeMB: 0},
		{Name: "loja_2024", SizeMB: 12.53},
		{Name: "sem_tabelas", SizeMB: 0},
	}
	if got := parseDatabases(out); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDatabases = %+v, want %+v", got, want)
	}
}

func TestParseDatabasesVazio(t *testing.T) {
	if got := parseDatabases(""); got != nil {
		t.Fatalf("parseDatabases(\"\") = %+v, want nil", got)
	}
}

// O comando que a tela Banco e o `hyphp db` mostram tem de abrir o cliente
// certo copiado e colado num terminal: caminho completo (o `mysql` do PATH
// pode ser de outra instalação), cliente do mesmo motor e aspas quando o
// caminho tem espaço (perfil "C:\Users\João Silva"), e cada valor separado
// da opção, porque o PowerShell parte `-h127.0.0.1` no primeiro ponto.
//
// Pastas e nome do cliente saem de filepath e sysproc.ExeName, os mesmos do
// código: no Windows o esperado é o caminho com "\" e o mysql.exe, no macOS
// o mesmo caminho com "/" e o binário sem sufixo.
func TestCommand(t *testing.T) {
	cliente := func(dir, nome string) string {
		return filepath.Join(filepath.FromSlash(dir), "bin", sysproc.ExeName(nome))
	}
	casos := []struct {
		nome string
		inst runtime.Installed
		pass string
		want string
	}{
		{"mysql", runtime.Installed{Kind: runtime.MySQL, Dir: filepath.FromSlash("C:/HyPHP/bin/mysql/mysql-8.4.11-winx64")}, "",
			cliente("C:/HyPHP/bin/mysql/mysql-8.4.11-winx64", "mysql") + " -u root -h 127.0.0.1 -P 3306"},
		{"mariadb usa o mariadb.exe", runtime.Installed{Kind: runtime.MariaDB, Dir: filepath.FromSlash("C:/HyPHP/bin/mariadb/mariadb-11.4.13-winx64")}, "",
			cliente("C:/HyPHP/bin/mariadb/mariadb-11.4.13-winx64", "mariadb") + " -u root -h 127.0.0.1 -P 3306"},
		{"caminho com espaço vai entre aspas", runtime.Installed{Kind: runtime.MySQL, Dir: filepath.FromSlash("C:/Users/João Silva/AppData/Local/HyPHP/bin/mysql/m")}, "",
			`"` + cliente("C:/Users/João Silva/AppData/Local/HyPHP/bin/mysql/m", "mysql") + `" -u root -h 127.0.0.1 -P 3306`},
		{"senha", runtime.Installed{Kind: runtime.MySQL, Dir: filepath.FromSlash("C:/m")}, "s3cr3t",
			cliente("C:/m", "mysql") + " -u root -ps3cr3t -h 127.0.0.1 -P 3306"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := New(c.inst, 3306).Command(c.pass); got != c.want {
				t.Errorf("Command() =\n  %s\nwant\n  %s", got, c.want)
			}
		})
	}
}
