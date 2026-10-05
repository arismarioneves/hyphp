package stack

import (
	"reflect"
	"strings"
	"testing"

	"hyphp/internal/netcfg"
	"hyphp/internal/runtime"
	"hyphp/internal/supervisor"
	"hyphp/internal/webserver"
)

// No Windows cada série tem PoolSize workers php-cgi, uma porta cada; no Mac
// é um php-fpm só (stack_darwin_test.go).
func TestDesiredTwoMajors(t *testing.T) {
	in := baseInput(proj("app81", osPath("C:/DEV/app81"), "8.1"), proj("app72", osPath("C:/DEV/app72"), "7.2"))
	out, err := desired(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Warnings) != 0 {
		t.Fatalf("warnings inesperados: %+v", out.Warnings)
	}

	// pools: um por major, ordenados por major, 4 portas contíguas e disjuntas
	if len(out.Pools) != 2 {
		t.Fatalf("pools = %d, want 2", len(out.Pools))
	}
	wantPools := map[string][]int{"php72": {9000, 9001, 9002, 9003}, "php81": {9004, 9005, 9006, 9007}}
	for _, p := range out.Pools {
		if !reflect.DeepEqual(p.Ports, wantPools[p.Name]) {
			t.Fatalf("pool %s ports = %v, want %v", p.Name, p.Ports, wantPools[p.Name])
		}
	}

	// 8 specs php:<major>:<i>
	got := specIDs(out.Specs, "php:")
	want := []string{"php:7.2:0", "php:7.2:1", "php:7.2:2", "php:7.2:3", "php:8.1:0", "php:8.1:1", "php:8.1:2", "php:8.1:3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("php specs = %v, want %v", got, want)
	}

	// receita C16 do worker
	for _, s := range out.Specs {
		if s.ID != "php:8.1:1" {
			continue
		}
		if s.Group != "php" || s.Port != 9005 {
			t.Fatalf("spec %+v", s)
		}
		if s.Exe != osPath("C:/rt/bin/php/php-8.1.10-Win32-vs16-x64/php-cgi.exe") {
			t.Fatalf("Exe = %q", s.Exe)
		}
		wantArgs := []string{"-b", "127.0.0.1:9005", "-c", osPath("C:/rt/etc/php/8.1/php.ini"), "-d", "cgi.force_redirect=0", "-d", "cgi.fix_pathinfo=1"}
		if !reflect.DeepEqual(s.Args, wantArgs) {
			t.Fatalf("Args = %q, want %q", s.Args, wantArgs)
		}
		if !reflect.DeepEqual(s.Env, []string{"PHP_FCGI_MAX_REQUESTS=0"}) {
			t.Fatalf("Env = %q", s.Env)
		}
		if p, ok := s.Probe.(*supervisor.TCPProbe); !ok || p.Addr != "127.0.0.1:9005" {
			t.Fatalf("Probe = %#v", s.Probe)
		}
		if !s.Restart.Enabled {
			t.Fatal("Restart deve estar ligado")
		}
	}

	// sites
	if len(out.Sites) != 2 {
		t.Fatalf("sites = %d", len(out.Sites))
	}
	byID := map[string]int{}
	for i, s := range out.Sites {
		byID[s.ID] = i
	}
	s81 := out.Sites[byID["app81"]]
	if s81.Domain != "app81.test" || s81.PoolName != "php81" || s81.Docroot != "C:/DEV/app81/public" {
		t.Fatalf("site app81 = %+v", s81)
	}
	if s81.TLSCert == "" || s81.TLSKey == "" || strings.Contains(s81.TLSCert, `\`) {
		t.Fatalf("TLS deve estar preenchido e com '/': %+v", s81)
	}
	if out.Sites[byID["app72"]].PoolName != "php72" {
		t.Fatalf("site app72 = %+v", out.Sites[byID["app72"]])
	}

	// alocador persistiu as duas chaves
	snap := in.Alloc.Snapshot()
	if len(snap["php:8.1"]) != 4 || len(snap["php:7.2"]) != 4 {
		t.Fatalf("snapshot = %v", snap)
	}

	// sem Web → nenhum spec web:
	if ids := specIDs(out.Specs, "web:"); len(ids) != 0 {
		t.Fatalf("não esperava spec web sem in.Web: %v", ids)
	}
}

func TestDesiredReleasesUnusedMajor(t *testing.T) {
	in := baseInput(proj("app81", osPath("C:/DEV/app81"), "8.1"))
	in.Alloc = netcfg.NewAllocatorWithProbe(9000, map[string][]int{"php:7.2": {9000, 9001, 9002, 9003}, "php:8.1": {9004, 9005, 9006, 9007}}, alwaysFree)
	if _, err := desired(in); err != nil {
		t.Fatal(err)
	}
	snap := in.Alloc.Snapshot()
	if _, still := snap["php:7.2"]; still {
		t.Fatalf("php:7.2 deveria ter sido liberado: %v", snap)
	}
	if !reflect.DeepEqual(snap["php:8.1"], []int{9004, 9005, 9006, 9007}) {
		t.Fatalf("php:8.1 deveria ser reutilizado: %v", snap)
	}
}

// No Windows etc/php/<série> continua só com o php.ini, sem sendmail_path nem
// socket do MySQL: o arquivo sai igual ao da 3.0.1 com ou sem Mailpit.
func TestPHPConfNoWindowsSoPHPIni(t *testing.T) {
	pool := webserver.PHPPool{Name: "php81", Version: "8.1", Ports: []int{9000, 9001}}
	files := phpConfFiles([]byte("ini"), pool, 2, osPath("C:/rt/log"))
	if len(files) != 1 || string(files["php.ini"]) != "ini" {
		t.Fatalf("arquivos = %v, want só php.ini", files)
	}
	if got := phpSendmail([]runtime.Installed{mailpitInstalled()}, 1025); got != "" {
		t.Fatalf("sendmail = %q, want vazio", got)
	}
	if got := mysqlSocket(osPath("C:/rt/var")); got != "" {
		t.Fatalf("socket = %q, want vazio", got)
	}
}

// No Windows o servidor e o --initialize-insecure seguem com --console (o log
// sai no stdout que o supervisor e o init capturam), e o mariadb-install-db.exe
// só recebe o datadir: é a ferramenta do Windows, não o script do Unix.
func TestArgsDoBancoNoWindows(t *testing.T) {
	in := baseInput()
	in.Runtimes = append(in.Runtimes, mysqlInstalled())
	sp, ok := mysqlSpec(in)
	if !ok {
		t.Fatal("esperava spec mysql")
	}
	ini := osPath("C:/rt/etc/mysql/my.ini")
	if want := []string{"--defaults-file=" + ini, "--console"}; !reflect.DeepEqual(sp.Args, want) {
		t.Fatalf("Args = %q, want %q", sp.Args, want)
	}

	exe, args := dbInits[runtime.MySQL].cmd(mysqlInstalled(), osPath("C:/rt/etc"), osPath("C:/rt/var/mysql-data"))
	if exe != osPath("C:/rt/bin/mysql/mysql-8.0.30-winx64/bin/mysqld.exe") {
		t.Fatalf("init MySQL exe = %q", exe)
	}
	if want := []string{"--defaults-file=" + ini, "--initialize-insecure", "--console"}; !reflect.DeepEqual(args, want) {
		t.Fatalf("init MySQL args = %q, want %q", args, want)
	}

	data := osPath("C:/rt/var/mariadb-data")
	exe, args = dbInits[runtime.MariaDB].cmd(mariadbInstalled("11.4.13"), osPath("C:/rt/etc"), data)
	if exe != osPath("C:/rt/bin/mariadb/mariadb-11.4.13-winx64/bin/mariadb-install-db.exe") {
		t.Fatalf("init MariaDB exe = %q", exe)
	}
	if want := []string{"--datadir=" + data}; !reflect.DeepEqual(args, want) {
		t.Fatalf("init MariaDB args = %q, want %q", args, want)
	}
}
