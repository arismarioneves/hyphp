package stack

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"hyphp/internal/netcfg"
	"hyphp/internal/project"
	"hyphp/internal/runtime"
	"hyphp/internal/state"
	"hyphp/internal/supervisor"
)

func php(version, major, dir string) runtime.Installed {
	return runtime.Installed{
		Kind: runtime.PHP, Version: version, Major: major, Dir: dir,
		Exe: filepath.Join(dir, "php.exe"), CGIExe: filepath.Join(dir, "php-cgi.exe"),
	}
}

func proj(id, root, phpMajor string) project.Project {
	return project.Project{
		Manifest:   project.Manifest{Name: id, Domain: id + ".test", PHP: phpMajor, Docroot: "public"},
		ID:         id,
		Root:       root,
		DocrootAbs: filepath.Join(root, "public"),
	}
}

func alwaysFree(int) bool { return true }

func fakeTLS(domains []string) (string, string, error) {
	return `C:\certs\` + domains[0] + ".pem", `C:\certs\` + domains[0] + "-key.pem", nil
}

func baseInput(projs ...project.Project) desiredInput {
	return desiredInput{
		State: state.State{
			WebServer: state.Apache, DefaultPHP: "8.1", PoolSize: 4, HTTPPort: 80, HTTPSPort: 443,
		},
		Runtimes: []runtime.Installed{
			php("8.1.10", "8.1", `C:\rt\bin\php\php-8.1.10-Win32-vs16-x64`),
			php("7.2.34", "7.2", `C:\rt\bin\php\php-7.2.34-Win32-VC15-x64`),
		},
		Projects: projs,
		Alloc:    netcfg.NewAllocatorWithProbe(9000, nil, alwaysFree),
		TLS:      fakeTLS,
		EtcDir:   `C:\rt\etc`,
		VarDir:   `C:\rt\var`,
		LogDir:   `C:\rt\log`,
	}
}

func specIDs(specs []supervisor.Spec, prefix string) []string {
	var ids []string
	for _, s := range specs {
		if strings.HasPrefix(s.ID, prefix) {
			ids = append(ids, s.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func TestDesiredTwoMajors(t *testing.T) {
	in := baseInput(proj("app81", `C:\DEV\app81`, "8.1"), proj("app72", `C:\DEV\app72`, "7.2"))
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
		if s.Exe != `C:\rt\bin\php\php-8.1.10-Win32-vs16-x64\php-cgi.exe` {
			t.Fatalf("Exe = %q", s.Exe)
		}
		wantArgs := []string{"-b", "127.0.0.1:9005", "-c", filepath.Join(`C:\rt\etc`, "php", "8.1", "php.ini"), "-d", "cgi.force_redirect=0", "-d", "cgi.fix_pathinfo=1"}
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

func TestDesiredMissingPHP(t *testing.T) {
	in := baseInput(proj("app81", `C:\DEV\app81`, "8.1"), proj("novo", `C:\DEV\novo`, "8.3"))
	out, err := desired(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Pools) != 1 || out.Pools[0].Name != "php81" {
		t.Fatalf("pools = %+v, want só php81", out.Pools)
	}
	if len(out.Sites) != 1 || out.Sites[0].ID != "app81" {
		t.Fatalf("sites = %+v", out.Sites)
	}
	if len(out.Warnings) != 1 || out.Warnings[0].Code != "php-missing" || out.Warnings[0].ProjectID != "novo" || !strings.Contains(out.Warnings[0].Message, "8.3") {
		t.Fatalf("warnings = %+v", out.Warnings)
	}
	if ids := specIDs(out.Specs, "php:"); len(ids) != 4 {
		t.Fatalf("php specs = %v, want 4", ids)
	}
}

func TestDesiredDefaultAndHighestPHP(t *testing.T) {
	t.Run("usa DefaultPHP", func(t *testing.T) {
		in := baseInput(proj("x", `C:\DEV\x`, ""))
		in.State.DefaultPHP = "7.2"
		out, err := desired(in)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Pools) != 1 || out.Pools[0].Version != "7.2" {
			t.Fatalf("pools = %+v", out.Pools)
		}
	})
	t.Run("sem DefaultPHP usa a maior instalada", func(t *testing.T) {
		in := baseInput(proj("x", `C:\DEV\x`, ""))
		in.State.DefaultPHP = ""
		out, err := desired(in)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Pools) != 1 || out.Pools[0].Version != "8.1" {
			t.Fatalf("pools = %+v", out.Pools)
		}
	})
}

func TestDesiredReleasesUnusedMajor(t *testing.T) {
	in := baseInput(proj("app81", `C:\DEV\app81`, "8.1"))
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

func TestDesiredHtaccessUnderNginx(t *testing.T) {
	p := proj("legacy", `C:\DEV\legacy`, "8.1")
	p.HasHtaccess = true
	in := baseInput(p)
	in.State.WebServer = state.Nginx
	out, err := desired(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Warnings) != 1 || out.Warnings[0].Code != "htaccess-under-nginx" || out.Warnings[0].ProjectID != "legacy" {
		t.Fatalf("warnings = %+v", out.Warnings)
	}
	if !out.Sites[0].HasHtaccess {
		t.Fatal("Site.HasHtaccess deve propagar")
	}

	// sob apache não há aviso
	in.State.WebServer = state.Apache
	out, _ = desired(in)
	if len(out.Warnings) != 0 {
		t.Fatalf("apache não deveria avisar: %+v", out.Warnings)
	}
}

func TestDesiredWildcardAndTLSFailure(t *testing.T) {
	p := proj("multi", `C:\DEV\multi`, "8.1")
	p.Wildcard = true
	in := baseInput(p)
	var gotDomains []string
	in.TLS = func(domains []string) (string, string, error) {
		gotDomains = domains
		return "", "", errTLS
	}
	out, err := desired(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotDomains, []string{"multi.test", "*.multi.test"}) {
		t.Fatalf("domínios pedidos ao TLS = %v", gotDomains)
	}
	s := out.Sites[0]
	if !reflect.DeepEqual(s.Aliases, []string{"*.multi.test"}) || s.TLSCert != "" {
		t.Fatalf("site = %+v", s)
	}
	if len(out.Warnings) != 1 || out.Warnings[0].Code != "tls-unavailable" || out.Warnings[0].ProjectID != "multi" {
		t.Fatalf("warnings = %+v", out.Warnings)
	}
}

func TestDesiredExtensionsUnion(t *testing.T) {
	a := proj("a", `C:\DEV\a`, "8.1")
	a.Extensions = []string{"intl", "pdo_mysql"}
	b := proj("b", `C:\DEV\b`, "8.1")
	b.Extensions = []string{"gd", "intl"}
	in := baseInput(a, b)
	in.State.PHPExtensions = map[string][]string{"8.1": {"mbstring", "gd"}}
	out, err := desired(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.Extensions["8.1"], []string{"gd", "intl", "mbstring", "pdo_mysql"}) {
		t.Fatalf("Extensions = %v", out.Extensions)
	}
}

func mysqlInstalled() runtime.Installed {
	dir := `C:\rt\bin\mysql\mysql-8.0.30-winx64`
	return runtime.Installed{
		Kind: runtime.MySQL, Version: "8.0.30", Major: "8.0.30", Dir: dir,
		Exe: filepath.Join(dir, "bin", "mysqld.exe"), Arch: "x64",
	}
}

func specByID(specs []supervisor.Spec, id string) (supervisor.Spec, bool) {
	for _, s := range specs {
		if s.ID == id {
			return s, true
		}
	}
	return supervisor.Spec{}, false
}

func TestDesiredMySQLSpec(t *testing.T) {
	in := baseInput(proj("app81", `C:\DEV\app81`, "8.1"))
	in.Runtimes = append(in.Runtimes, mysqlInstalled())
	in.State.MySQLPort = 3307 // a porta vem do state, nunca é constante

	out, err := desired(in)
	if err != nil {
		t.Fatal(err)
	}
	sp, ok := specByID(out.Specs, "mysql")
	if !ok {
		t.Fatal("esperava spec mysql")
	}
	if sp.Group != "db" || sp.Port != 3307 {
		t.Fatalf("spec = %+v", sp)
	}
	if sp.Exe != `C:\rt\bin\mysql\mysql-8.0.30-winx64\bin\mysqld.exe` {
		t.Fatalf("Exe = %q", sp.Exe)
	}
	wantArgs := []string{`--defaults-file=C:\rt\etc\mysql\my.ini`, "--console"}
	if !reflect.DeepEqual(sp.Args, wantArgs) {
		t.Fatalf("Args = %q, want %q", sp.Args, wantArgs)
	}
	p, ok := sp.Probe.(*supervisor.MySQLProbe)
	if !ok || p.Addr != "127.0.0.1:3307" {
		t.Fatalf("Probe = %#v", sp.Probe)
	}
	if sp.ProbeTimeout != 60*time.Second {
		t.Fatalf("ProbeTimeout = %s, want 60s", sp.ProbeTimeout)
	}
	if !sp.Restart.Enabled {
		t.Fatal("Restart deve estar ligado")
	}
	if sp.LogPath != `C:\rt\log\mysql.log` {
		t.Fatalf("LogPath = %q", sp.LogPath)
	}
}

// Instalar um MySQL mais antigo não pode trocar o banco em uso. A pasta do
// 8.0 vem antes da do 8.4 na ordem de bin/, e era a primeira da lista que
// subia: o 8.0 recusa o datadir do 8.4 ("downgrade is only permitted between
// patch releases") e o serviço entra em loop de restart.
func TestDesiredMySQLUsaAVersaoMaisNova(t *testing.T) {
	antigo := `C:\rt\bin\mysql\mysql-8.0.46-winx64`
	novo := `C:\rt\bin\mysql\mysql-8.4.11-winx64`
	in := baseInput(proj("app81", `C:\DEV\app81`, "8.1"))
	in.Runtimes = append(in.Runtimes,
		runtime.Installed{Kind: runtime.MySQL, Version: "8.0.46", Major: "8.0.46", Dir: antigo, Exe: filepath.Join(antigo, "bin", "mysqld.exe")},
		runtime.Installed{Kind: runtime.MySQL, Version: "8.4.11", Major: "8.4.11", Dir: novo, Exe: filepath.Join(novo, "bin", "mysqld.exe")},
	)
	out, err := desired(in)
	if err != nil {
		t.Fatal(err)
	}
	sp, ok := specByID(out.Specs, "mysql")
	if !ok || sp.Exe != filepath.Join(novo, "bin", "mysqld.exe") {
		t.Fatalf("spec mysql = %q, quero o 8.4.11", sp.Exe)
	}
}
func mariadbInstalled(version string) runtime.Installed {
	dir := `C:\rt\bin\mariadb\mariadb-` + version + `-winx64`
	return runtime.Installed{Kind: runtime.MariaDB, Version: version, Major: version, Dir: dir, Exe: filepath.Join(dir, "bin", "mariadbd.exe")}
}

// O motor escolhido manda: com MariaDB em DBEngine, o spec "mysql" roda o
// mariadbd mesmo com um MySQL instalado ao lado; sem escolha, vale o MySQL.
func TestDesiredMotorDeBanco(t *testing.T) {
	casos := []struct {
		nome    string
		engine  string
		rts     []runtime.Installed
		wantExe string
		wantNom string
	}{
		{"mariadb escolhido", state.DBMariaDB, []runtime.Installed{mysqlInstalled(), mariadbInstalled("10.11.19"), mariadbInstalled("11.4.13")},
			`C:\rt\bin\mariadb\mariadb-11.4.13-winx64\bin\mariadbd.exe`, "MariaDB 11.4.13"},
		{"sem escolha, os dois instalados", "", []runtime.Installed{mariadbInstalled("11.4.13"), mysqlInstalled()},
			`C:\rt\bin\mysql\mysql-8.0.30-winx64\bin\mysqld.exe`, "MySQL 8.0.30"},
		{"sem escolha, só o MariaDB", "", []runtime.Installed{mariadbInstalled("11.4.13")},
			`C:\rt\bin\mariadb\mariadb-11.4.13-winx64\bin\mariadbd.exe`, "MariaDB 11.4.13"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			in := baseInput(proj("app81", `C:\DEV\app81`, "8.1"))
			in.Runtimes = append(in.Runtimes, c.rts...)
			in.State.DBEngine = c.engine
			out, err := desired(in)
			if err != nil {
				t.Fatal(err)
			}
			sp, ok := specByID(out.Specs, MySQLSpecID)
			if !ok || sp.Exe != c.wantExe || sp.Name != c.wantNom {
				t.Fatalf("spec = %q %q, quero %q %q", sp.Name, sp.Exe, c.wantNom, c.wantExe)
			}
		})
	}
}

// MariaDB escolhido e não instalado: nenhum banco sobe. Cair no MySQL
// mostraria os databases de outro servidor sem o usuário ter pedido.
func TestDesiredMotorEscolhidoAusente(t *testing.T) {
	in := baseInput(proj("app81", `C:\DEV\app81`, "8.1"))
	in.Runtimes = append(in.Runtimes, mysqlInstalled())
	in.State.DBEngine = state.DBMariaDB
	out, err := desired(in)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := specByID(out.Specs, MySQLSpecID); ok {
		t.Fatal("subiu um banco com o motor escolhido ausente")
	}
}

func TestDesiredSemMySQL(t *testing.T) {
	out, err := desired(baseInput(proj("app81", `C:\DEV\app81`, "8.1")))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := specByID(out.Specs, "mysql"); ok {
		t.Fatal("sem runtime MySQL não pode haver spec mysql")
	}
	if len(out.Warnings) != 0 {
		t.Fatalf("MySQL é opcional; nenhum warning esperado: %+v", out.Warnings)
	}
}

func mailpitInstalled() runtime.Installed {
	dir := `C:\rt\bin\mailpit`
	return runtime.Installed{
		Kind: runtime.Mailpit, Version: "1.31.1", Major: "1.31.1", Dir: dir,
		Exe: filepath.Join(dir, "mailpit.exe"),
	}
}

func TestDesiredMailpitSpec(t *testing.T) {
	in := baseInput(proj("app81", `C:\DEV\app81`, "8.1"))
	in.Runtimes = append(in.Runtimes, mailpitInstalled())
	in.State.MailpitSMTPPort = 1026
	in.State.MailpitHTTPPort = 8026

	out, err := desired(in)
	if err != nil {
		t.Fatal(err)
	}
	sp, ok := specByID(out.Specs, "mailpit")
	if !ok {
		t.Fatal("esperava spec mailpit")
	}
	if sp.Group != "mail" || sp.Port != 8026 {
		t.Fatalf("spec = %+v", sp)
	}
	if sp.Exe != `C:\rt\bin\mailpit\mailpit.exe` {
		t.Fatalf("Exe = %q", sp.Exe)
	}
	wantArgs := []string{
		"--smtp", "127.0.0.1:1026",
		"--listen", "127.0.0.1:8026",
		"--database", `C:\rt\var\mailpit.db`,
	}
	if !reflect.DeepEqual(sp.Args, wantArgs) {
		t.Fatalf("Args = %q, want %q", sp.Args, wantArgs)
	}
	p, ok := sp.Probe.(*supervisor.TCPProbe)
	if !ok || p.Addr != "127.0.0.1:8026" {
		t.Fatalf("Probe = %#v", sp.Probe)
	}
	if !sp.Restart.Enabled || sp.LogPath != `C:\rt\log\mailpit.log` {
		t.Fatalf("spec = %+v", sp)
	}
}

func TestDesiredSemMailpit(t *testing.T) {
	out, err := desired(baseInput(proj("app81", `C:\DEV\app81`, "8.1")))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := specByID(out.Specs, "mailpit"); ok {
		t.Fatal("sem runtime Mailpit não pode haver spec mailpit")
	}
}

// stubLookPath troca a resolução de executáveis por uma tabela fixa, para o
// teste não depender do PATH da máquina.
func stubLookPath(t *testing.T, table map[string]string) func() {
	t.Helper()
	old := lookPathIn
	lookPathIn = func(_ string, file string) (string, error) {
		if p, ok := table[file]; ok {
			return p, nil
		}
		return "", fmt.Errorf("stack: %s não encontrado no PATH", file)
	}
	return func() { lookPathIn = old }
}

func TestDesiredProcesses(t *testing.T) {
	restore := stubLookPath(t, map[string]string{
		"npm": `C:\Program Files\nodejs\npm.cmd`,
	})
	defer restore()

	p := proj("app81", `C:\DEV\app81`, "8.1")
	p.Processes = map[string]string{
		"queue":     "php artisan queue:work --tries=3",
		"scheduler": "php artisan schedule:work",
		"vite":      `npm run dev -- --host "127.0.0.1"`,
	}
	out, err := desired(baseInput(p))
	if err != nil {
		t.Fatal(err)
	}
	procs := map[string]supervisor.Spec{}
	for _, s := range out.Specs {
		if strings.HasPrefix(s.ID, "proc:") {
			procs[s.ID] = s
		}
	}
	if len(procs) != 3 {
		t.Fatalf("procs = %v", procs)
	}
	q := procs["proc:app81:queue"]
	if q.Group != "proc" || q.Dir != `C:\DEV\app81` {
		t.Fatalf("queue = %+v", q)
	}
	if q.Exe != `C:\rt\bin\php\php-8.1.10-Win32-vs16-x64\php.exe` {
		t.Fatalf("php deve resolver para o php.exe do major: %q", q.Exe)
	}
	if !reflect.DeepEqual(q.Args, []string{"artisan", "queue:work", "--tries=3"}) {
		t.Fatalf("Args = %q", q.Args)
	}
	if len(q.Env) != 1 || !strings.HasPrefix(q.Env[0], `PATH=C:\rt\bin\php\php-8.1.10-Win32-vs16-x64;`) {
		t.Fatalf("Env = %q", q.Env)
	}
	if ap, ok := q.Probe.(*supervisor.AliveProbe); !ok || ap.Grace != 2*time.Second {
		t.Fatalf("Probe = %#v", q.Probe)
	}
	if !q.Restart.Enabled || q.Restart.MaxRetries != 0 || q.Restart.BaseDelay != time.Second || q.Restart.MaxDelay != 30*time.Second {
		t.Fatalf("Restart = %+v", q.Restart)
	}
	if q.LogPath != `C:\rt\log\proc-app81-queue.log` {
		t.Fatalf("LogPath = %q", q.LogPath)
	}
	v := procs["proc:app81:vite"]
	if v.Exe != `C:\Program Files\nodejs\npm.cmd` {
		t.Fatalf("vite.Exe = %q", v.Exe)
	}
	if !reflect.DeepEqual(v.Args, []string{"run", "dev", "--", "--host", "127.0.0.1"}) {
		t.Fatalf("vite.Args = %q", v.Args)
	}
	if len(out.Warnings) != 0 {
		t.Fatalf("warnings = %+v", out.Warnings)
	}
}

func TestDesiredProcExeMissing(t *testing.T) {
	restore := stubLookPath(t, nil)
	defer restore()

	p := proj("app81", `C:\DEV\app81`, "8.1")
	p.Processes = map[string]string{"vite": "npm run dev"}
	out, err := desired(baseInput(p))
	if err != nil {
		t.Fatal(err)
	}
	if ids := specIDs(out.Specs, "proc:"); len(ids) != 0 {
		t.Fatalf("não pode criar spec com exe inexistente: %v", ids)
	}
	if len(out.Warnings) != 1 || out.Warnings[0].Code != "proc-exe-missing" || out.Warnings[0].ProjectID != "app81" {
		t.Fatalf("warnings = %+v", out.Warnings)
	}
	if !strings.Contains(out.Warnings[0].Message, "npm") {
		t.Fatalf("mensagem = %q", out.Warnings[0].Message)
	}
}

func TestSplitCommand(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"artisan", "php artisan queue:work --tries=3", []string{"php", "artisan", "queue:work", "--tries=3"}},
		{"espaços extras", "  a   b  ", []string{"a", "b"}},
		{"caminho entre aspas", `node "C:\Program Files\x.js" --flag`, []string{"node", `C:\Program Files\x.js`, "--flag"}},
		{"barras preservadas sem aspas", `cmd C:\temp\a.txt`, []string{"cmd", `C:\temp\a.txt`}},
		{"aspas simples", `x 'single quoted arg' y`, []string{"x", "single quoted arg", "y"}},
		{"aspas escapadas", `php -r "echo \"oi\";"`, []string{"php", "-r", `echo "oi";`}},
		{"barra dupla antes de aspas", `a "C:\dir\\" b`, []string{"a", `C:\dir\`, "b"}},
		{"argumento vazio", `a "" b`, []string{"a", "", "b"}},
		{"vazio", "", nil},
		{"só espaços", "   ", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := splitCommand(c.in); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("splitCommand(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestSplitCommandRoundTrip prova que splitCommand é o inverso de
// syscall.EscapeArg — o mesmo escape que o os/exec usa para montar a linha de
// comando no Windows. É o teste que pega regressão de contagem de barras.
func TestSplitCommandRoundTrip(t *testing.T) {
	cases := [][]string{
		{"php", "artisan", "queue:work", "--tries=3"},
		{"node", `C:\Program Files\nodejs\x.js`, "--flag"},
		{"cmd", `C:\temp\`, "fim"},
		{"echo", `aspas "no meio"`},
		{"echo", ""},
		{"a b", "c\td"},
		{`barra\dupla\\`, "x"},
	}
	for _, args := range cases {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = syscall.EscapeArg(a)
		}
		line := strings.Join(parts, " ")
		if got := splitCommand(line); !reflect.DeepEqual(got, args) {
			t.Fatalf("splitCommand(%q) = %q, want %q", line, got, args)
		}
	}
}

func TestDefaultLookPathIn(t *testing.T) {
	dir := t.TempDir()
	bat := filepath.Join(dir, "tool.bat")
	if err := os.WriteFile(bat, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := defaultLookPathIn(dir, "tool")
	if err != nil || got != bat {
		t.Fatalf("defaultLookPathIn = %q, %v; want %q", got, err, bat)
	}
	if _, err := defaultLookPathIn(dir, "inexistente"); err == nil {
		t.Fatal("esperava erro para arquivo inexistente")
	}
	if got, err := defaultLookPathIn("", bat); err != nil || got != bat {
		t.Fatalf("caminho absoluto = %q, %v", got, err)
	}
}

// O retrato de State() vai para o Reconcile, que itera os mapas sem lock.
// Qualquer mapa compartilhado com o state vivo vira escrita concorrente
// quando o usuário edita uma diretiva no meio do Reconcile — e isso derruba o
// processo, não só a goroutine.
func TestCloneStateNaoCompartilhaMapas(t *testing.T) {
	orig := state.Default()
	orig.PortAlloc["php:8.3"] = []int{9000}
	orig.PHPExtensions = map[string][]string{"8.3": {"curl"}}
	orig.PHPIni = map[string]map[string]string{"8.3": {"memory_limit": "256M"}}

	c := cloneState(orig)
	c.PortAlloc["php:8.3"][0] = 1
	c.PHPExtensions["8.3"][0] = "x"
	c.PHPIni["8.3"]["memory_limit"] = "1G"
	c.PHPIni["8.3"]["max_input_vars"] = "5000"

	if orig.PortAlloc["php:8.3"][0] != 9000 {
		t.Error("PortAlloc compartilhado com a cópia")
	}
	if orig.PHPExtensions["8.3"][0] != "curl" {
		t.Error("PHPExtensions compartilhado com a cópia")
	}
	if got := orig.PHPIni["8.3"]; len(got) != 1 || got["memory_limit"] != "256M" {
		t.Errorf("PHPIni compartilhado com a cópia: %v", got)
	}
}
