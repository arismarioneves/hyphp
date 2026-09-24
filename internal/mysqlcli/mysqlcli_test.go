package mysqlcli

import (
	"reflect"
	"strings"
	"testing"
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
