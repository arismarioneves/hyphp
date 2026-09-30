package stack

import (
	"reflect"
	"testing"

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
