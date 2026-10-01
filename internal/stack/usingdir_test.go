package stack

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"hyphp/internal/supervisor"
)

// Remover um runtime para os serviços que rodam dele, e só esses: parar o
// MySQL errado derrubaria o banco em uso.
func TestSpecsUsingDirSoOsDaquelaPasta(t *testing.T) {
	specs := []supervisor.Spec{
		{ID: "mysql", Exe: `C:\HyPHP\bin\mysql\mysql-8.0.46-winx64\bin\mysqld.exe`},
		{ID: "php:8.1:1", Exe: `C:\HyPHP\bin\php\php-8.1.34\php-cgi.exe`},
		{ID: "php:8.1:0", Exe: `C:\HyPHP\bin\php\php-8.1.34\php-cgi.exe`},
		{ID: "velho", Exe: `C:\HyPHP\bin\mysql\mysql-8.0.46-winx64-old\bin\mysqld.exe`},
		{ID: "web:apache", Exe: `C:\HyPHP\bin\apache\httpd-2.4.68\bin\httpd.exe`},
	}
	if got := specsUsingDir(specs, `c:\hyphp\bin\mysql\mysql-8.0.46-winx64`); !reflect.DeepEqual(got, []string{"mysql"}) {
		t.Errorf("mysql 8.0.46: %v", got)
	}
	if got := specsUsingDir(specs, `C:\HyPHP\bin\php\php-8.1.34\`); !reflect.DeepEqual(got, []string{"php:8.1:0", "php:8.1:1"}) {
		t.Errorf("php 8.1: %v", got)
	}
	if got := specsUsingDir(specs, `C:\HyPHP\bin\nginx\nginx-1.30.5`); got != nil {
		t.Errorf("pasta sem serviço: %v", got)
	}
}

// Remover uma versão com outra do mesmo tipo instalada mantém o ID do spec e
// muda só o executável. O serviço tem de voltar na versão que ficou: era o
// StopUsingDir, e não o usuário, quem o tinha parado.
func TestTrocaDeVersaoDepoisDeStopUsingDirReligaOServico(t *testing.T) {
	sup, err := supervisor.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("supervisor.New: %v", err)
	}
	t.Cleanup(func() { _ = sup.Close() })
	s := New(Deps{Sup: sup, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	cmdExe := os.Getenv("ComSpec")
	if cmdExe == "" {
		t.Skip("ComSpec ausente")
	}
	spec := func(args ...string) supervisor.Spec {
		return supervisor.Spec{
			ID: MySQLSpecID, Group: "db", Exe: cmdExe, Args: args,
			Probe: supervisor.AliveProbe{Grace: 300 * time.Millisecond},
		}
	}
	if err := s.applySpecs([]supervisor.Spec{spec("/c", "ping -n 30 127.0.0.1 >nul")}, false, nil, false); err != nil {
		t.Fatalf("applySpecs: %v", err)
	}
	if err := s.StartAll(t.Context()); err != nil {
		t.Fatalf("StartAll: %v", err)
	}

	if err := s.StopUsingDir(filepath.Dir(cmdExe)); err != nil {
		t.Fatalf("StopUsingDir: %v", err)
	}
	if s.isRunning(MySQLSpecID) {
		t.Fatal("StopUsingDir não parou o serviço")
	}
	// A revarredura troca o spec (aqui, os argumentos fazem as vezes do
	// executável da outra versão).
	if err := s.applySpecs([]supervisor.Spec{spec("/c", "ping -n 31 127.0.0.1 >nul")}, false, nil, false); err != nil {
		t.Fatalf("applySpecs com o spec trocado: %v", err)
	}
	if !s.isRunning(MySQLSpecID) {
		st, _ := sup.Status(MySQLSpecID)
		t.Fatalf("serviço ficou %q depois da troca de versão; quero no ar", st.State)
	}
}
