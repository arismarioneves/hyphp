package stack

import (
	"reflect"
	"strings"
	"testing"

	"hyphp/internal/runtime"
	"hyphp/internal/supervisor"
	"hyphp/internal/webserver"
)

// No Mac cada série é um php-fpm só, com uma porta: o ID continua
// php:<série>:0 para o reinício por php.ini, os pools da UI e specsUsingDir.
func TestSpecDoFPMNoMac(t *testing.T) {
	in := baseInput(proj("app81", "/Users/dev/Code/app81", "8.1"), proj("app72", "/Users/dev/Code/app72", "7.2"))
	in.Runtimes = []runtime.Installed{
		{Kind: runtime.PHP, Version: "8.1.33", Major: "8.1", Dir: "/opt/homebrew/opt/php@8.1",
			Exe: "/opt/homebrew/opt/php@8.1/bin/php", CGIExe: "/opt/homebrew/opt/php@8.1/sbin/php-fpm"},
		{Kind: runtime.PHP, Version: "7.2.34", Major: "7.2", Dir: "/opt/homebrew/opt/php@7.2",
			Exe: "/opt/homebrew/opt/php@7.2/bin/php", CGIExe: "/opt/homebrew/opt/php@7.2/sbin/php-fpm"},
	}
	in.EtcDir = "/Users/dev/Library/Application Support/HyPHP/etc"
	in.LogDir = "/Users/dev/Library/Application Support/HyPHP/log"
	out, err := desired(in)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := specIDs(out.Specs, "php:"), []string{"php:7.2:0", "php:8.1:0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("php specs = %v, want %v", got, want)
	}
	wantPools := map[string][]int{"php72": {9000}, "php81": {9001}}
	for _, p := range out.Pools {
		if !reflect.DeepEqual(p.Ports, wantPools[p.Name]) {
			t.Fatalf("pool %s ports = %v, want %v", p.Name, p.Ports, wantPools[p.Name])
		}
	}
	if snap := in.Alloc.Snapshot(); len(snap["php:8.1"]) != 1 || len(snap["php:7.2"]) != 1 {
		t.Fatalf("PoolSize 4 no Mac reserva uma porta por série: %v", snap)
	}

	var s supervisor.Spec
	for _, sp := range out.Specs {
		if sp.ID == "php:8.1:0" {
			s = sp
		}
	}
	if s.Group != "php" || s.Port != 9001 || s.Dir != "/opt/homebrew/opt/php@8.1" {
		t.Fatalf("spec %+v", s)
	}
	if s.Exe != "/opt/homebrew/opt/php@8.1/sbin/php-fpm" {
		t.Fatalf("Exe = %q", s.Exe)
	}
	wantArgs := []string{
		"--nodaemonize", "--force-stderr",
		"--fpm-config", "/Users/dev/Library/Application Support/HyPHP/etc/php/8.1/php-fpm.conf",
		"-c", "/Users/dev/Library/Application Support/HyPHP/etc/php/8.1/php.ini",
	}
	if !reflect.DeepEqual(s.Args, wantArgs) {
		t.Fatalf("Args = %q, want %q", s.Args, wantArgs)
	}
	if !reflect.DeepEqual(s.Env, []string{"PHP_INI_SCAN_DIR="}) {
		t.Fatalf("Env = %q", s.Env)
	}
	if p, ok := s.Probe.(*supervisor.TCPProbe); !ok || p.Addr != "127.0.0.1:9001" {
		t.Fatalf("Probe = %#v", s.Probe)
	}
	if !s.Restart.Enabled {
		t.Fatal("Restart deve estar ligado")
	}
	if s.LogPath != "/Users/dev/Library/Application Support/HyPHP/log/php-8.1-0.log" {
		t.Fatalf("LogPath = %q", s.LogPath)
	}
}

// php.ini e php-fpm.conf saem no mesmo WriteFiles: a varredura apagaria um
// arquivo gravado à parte, e mudar só o PoolSize tem de mudar o conjunto
// (é isso que reinicia a série).
func TestPHPConfNoMacTrazFPMConf(t *testing.T) {
	pool := webserver.PHPPool{Name: "php83", Version: "8.3", Ports: []int{9004}}
	files := phpConfFiles([]byte("ini"), pool, 4, "/log")
	if string(files["php.ini"]) != "ini" || len(files) != 2 {
		t.Fatalf("arquivos = %v", files)
	}
	fpm := string(files["php-fpm.conf"])
	if !strings.Contains(fpm, "\nlisten = 127.0.0.1:9004\n") || !strings.Contains(fpm, "\npm.max_children = 4\n") {
		t.Fatalf("php-fpm.conf:\n%s", fpm)
	}
	if outro := phpConfFiles([]byte("ini"), pool, 2, "/log"); string(outro["php-fpm.conf"]) == fpm {
		t.Fatal("PoolSize diferente devia mudar o php-fpm.conf")
	}
}

// mail() no Mac vai pelo sendmail do Mailpit; sem Mailpit, um comando que
// falha, para nunca cair no Postfix e sair para o mundo.
func TestSendmailNoMac(t *testing.T) {
	mp := runtime.Installed{Kind: runtime.Mailpit, Version: "1.31.1", Major: "1.31.1",
		Dir: "/opt/homebrew/opt/mailpit", Exe: "/opt/homebrew/opt/mailpit/bin/mailpit"}
	if got, want := phpSendmail([]runtime.Installed{mp}, 1026), `"/opt/homebrew/opt/mailpit/bin/mailpit" sendmail -S 127.0.0.1:1026`; got != want {
		t.Fatalf("sendmail = %q, want %q", got, want)
	}
	if got := phpSendmail(nil, 1026); got != "/usr/bin/false" {
		t.Fatalf("sem Mailpit sendmail = %q, want /usr/bin/false", got)
	}
	if got, want := phpMySQLSocket("/Users/dev/Library/Application Support/HyPHP/var"), "/Users/dev/Library/Application Support/HyPHP/var/run/mysql.sock"; got != want {
		t.Fatalf("socket = %q, want %q", got, want)
	}
}
