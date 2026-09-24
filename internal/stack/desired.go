package stack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"hyphp/internal/netcfg"
	"hyphp/internal/project"
	"hyphp/internal/runtime"
	"hyphp/internal/state"
	"hyphp/internal/supervisor"
	"hyphp/internal/webserver"
)

// groupOrder é a ordem de StartAll; StopAll usa a inversa. PHP antes do web
// server para o balancer já encontrar workers; procs por último porque
// dependem de banco/mail (planos 07/08 acrescentam "db" e "mail" aqui no meio).
var groupOrder = []string{"php", "web", "db", "mail", "proc"}

// errTLS é usado nos testes para simular falha de emissão.
var errTLS = errors.New("tls indisponível")

// desiredInput é tudo que desired precisa. Alloc é mutado (Reserve/Release);
// Web e TLS podem ser nil.
type desiredInput struct {
	State    state.State
	Runtimes []runtime.Installed
	Projects []project.Project
	Alloc    *netcfg.Allocator
	Web      webserver.WebServer                                  // nil = sem spec web
	TLS      func(domains []string) (cert, key string, err error) // nil = sem TLS
	EtcDir   string
	VarDir   string
	LogDir   string
}

// desiredOutput é o estado-alvo. Specs contém php:*, web:* e proc:*; o plano 07
// acrescenta mysql e mailpit.
type desiredOutput struct {
	Specs      []supervisor.Spec
	Sites      []webserver.Site
	Pools      []webserver.PHPPool
	Extensions map[string][]string // major → extensões habilitadas (state ∪ manifestos)
	Warnings   []Warning
}

type served struct {
	p    project.Project
	inst runtime.Installed
}

// desired é pura exceto pelo alocador (que persiste via Snapshot) e por
// os.Getenv("PATH") nos procs. Nunca toca no supervisor nem no disco.
func desired(in desiredInput) (desiredOutput, error) {
	out := desiredOutput{Extensions: map[string][]string{}}
	phps := runtime.ByKind(in.Runtimes, runtime.PHP)

	// 1. resolver o major de cada projeto: manifesto → DefaultPHP → maior instalada
	var srv []served
	byMajor := map[string][]project.Project{}
	for _, p := range in.Projects {
		major := p.PHP
		if major == "" {
			major = in.State.DefaultPHP
		}
		if major == "" {
			major = highestMajor(phps)
		}
		inst, ok := runtime.PHPByMajor(phps, major)
		if !ok {
			out.Warnings = append(out.Warnings, Warning{
				Code:      "php-missing",
				ProjectID: p.ID,
				Message:   fmt.Sprintf("PHP %s não está instalado; %s não será servido", major, p.ID),
			})
			continue
		}
		srv = append(srv, served{p: p, inst: inst})
		byMajor[inst.Major] = append(byMajor[inst.Major], p)
	}

	// 2. pools: só majors com projeto (spec §15.5); libera ranges órfãos (§6.3)
	for key := range in.Alloc.Snapshot() {
		major, isPHP := strings.CutPrefix(key, "php:")
		if isPHP && byMajor[major] == nil {
			in.Alloc.Release(key)
		}
	}
	majors := make([]string, 0, len(byMajor))
	for m := range byMajor {
		majors = append(majors, m)
	}
	sort.Strings(majors)
	for _, major := range majors {
		inst, _ := runtime.PHPByMajor(phps, major)
		ports, err := in.Alloc.Reserve("php:"+major, in.State.PoolSize)
		if err != nil {
			return out, fmt.Errorf("stack: reservar %d portas para PHP %s: %w", in.State.PoolSize, major, err)
		}
		out.Pools = append(out.Pools, webserver.PHPPool{Name: webserver.PoolName(major), Version: major, Ports: ports})
		out.Extensions[major] = unionSorted(in.State.PHPExtensions[major], byMajor[major])
		for i, port := range ports {
			out.Specs = append(out.Specs, phpWorkerSpec(inst, major, i, port, in.EtcDir, in.LogDir))
		}
	}

	// 3. sites (um por projeto servido)
	for _, s := range srv {
		site, warns := siteFor(in, s.p, s.inst.Major)
		out.Sites = append(out.Sites, site)
		out.Warnings = append(out.Warnings, warns...)
	}

	// 4. web server
	if in.Web != nil {
		out.Specs = append(out.Specs, webSpec(in.Web, in.State, in.EtcDir, in.LogDir))
	}

	// 5. processes dos projetos
	for _, s := range srv {
		out.Specs = append(out.Specs, procSpecs(s.p, s.inst, in.LogDir)...)
	}
	return out, nil
}

// highestMajor devolve o maior "major.minor" instalado ("" se nenhum).
// Comparação numérica por componente, não lexicográfica (8.10 > 8.9).
func highestMajor(phps []runtime.Installed) string {
	best := ""
	var bm, bn int
	for _, p := range phps {
		var m, n int
		if _, err := fmt.Sscanf(p.Major, "%d.%d", &m, &n); err != nil {
			continue
		}
		if best == "" || m > bm || (m == bm && n > bn) {
			best, bm, bn = p.Major, m, n
		}
	}
	return best
}

func unionSorted(global []string, projs []project.Project) []string {
	set := map[string]bool{}
	for _, e := range global {
		set[e] = true
	}
	for _, p := range projs {
		for _, e := range p.Extensions {
			set[e] = true
		}
	}
	out := make([]string, 0, len(set))
	for e := range set {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

func defaultRestart() supervisor.RestartPolicy {
	return supervisor.RestartPolicy{Enabled: true, MaxRetries: 0, BaseDelay: time.Second, MaxDelay: 30 * time.Second}
}

// phpWorkerSpec é a receita C16: php-cgi em modo FastCGI externo, sem limite
// de requests por processo (o default 500 mataria o worker sob carga).
func phpWorkerSpec(inst runtime.Installed, major string, i, port int, etcDir, logDir string) supervisor.Spec {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	return supervisor.Spec{
		ID:    fmt.Sprintf("php:%s:%d", major, i),
		Name:  fmt.Sprintf("PHP %s #%d", major, i),
		Group: "php",
		Exe:   inst.CGIExe,
		Args: []string{
			"-b", addr,
			"-c", filepath.Join(etcDir, "php", major, "php.ini"),
			"-d", "cgi.force_redirect=0",
			"-d", "cgi.fix_pathinfo=1",
		},
		Env:     []string{"PHP_FCGI_MAX_REQUESTS=0"},
		Dir:     inst.Dir,
		Port:    port,
		Probe:   &supervisor.TCPProbe{Addr: addr},
		Restart: defaultRestart(),
		LogPath: filepath.Join(logDir, fmt.Sprintf("php-%s-%d.log", major, i)),
	}
}

func siteFor(in desiredInput, p project.Project, major string) (webserver.Site, []Warning) {
	var warns []Warning
	site := webserver.Site{
		ID:          p.ID,
		Domain:      p.Domain,
		Docroot:     filepath.ToSlash(p.DocrootAbs),
		PoolName:    webserver.PoolName(major),
		HasHtaccess: p.HasHtaccess,
	}
	domains := []string{p.Domain}
	if p.Wildcard {
		site.Aliases = []string{"*." + p.Domain}
		domains = append(domains, "*."+p.Domain)
	}
	if in.TLS != nil {
		cert, key, err := in.TLS(domains)
		if err != nil {
			warns = append(warns, Warning{
				Code: "tls-unavailable", ProjectID: p.ID,
				Message: fmt.Sprintf("sem HTTPS para %s: %v", p.Domain, err),
			})
		} else {
			site.TLSCert = filepath.ToSlash(cert)
			site.TLSKey = filepath.ToSlash(key)
		}
	}
	if in.State.WebServer == state.Nginx && p.HasHtaccess {
		warns = append(warns, Warning{
			Code: "htaccess-under-nginx", ProjectID: p.ID,
			Message: fmt.Sprintf("%s tem .htaccess; rewrites não se aplicam sob nginx", p.ID),
		})
	}
	return site, warns
}

func webSpec(w webserver.WebServer, st state.State, etcDir, logDir string) supervisor.Spec {
	name := string(w.Name())
	exe, args, dir := w.Command(filepath.Join(etcDir, name))
	return supervisor.Spec{
		ID:           "web:" + name,
		Name:         strings.ToUpper(name[:1]) + name[1:],
		Group:        "web",
		Exe:          exe,
		Args:         args,
		Dir:          dir,
		Port:         st.HTTPPort,
		Probe:        w.Probe(webserver.Ports{HTTP: st.HTTPPort, HTTPS: st.HTTPSPort}),
		ProbeTimeout: 20 * time.Second,
		Restart:      defaultRestart(),
		LogPath:      filepath.Join(logDir, name+".log"),
	}
}

// procSpecs gera um spec por entrada de processes (C15): Dir = Root, `php`
// resolvido para o php.exe do major do projeto, PATH com a pasta do PHP na frente.
func procSpecs(p project.Project, inst runtime.Installed, logDir string) []supervisor.Spec {
	names := make([]string, 0, len(p.Processes))
	for n := range p.Processes {
		names = append(names, n)
	}
	sort.Strings(names)
	specs := make([]supervisor.Spec, 0, len(names))
	for _, name := range names {
		tokens := splitArgs(p.Processes[name])
		if len(tokens) == 0 {
			continue // Validate já impede; defesa contra manifesto editado à mão
		}
		exe := tokens[0]
		if strings.EqualFold(exe, "php") || strings.EqualFold(exe, "php.exe") {
			exe = inst.Exe
		}
		specs = append(specs, supervisor.Spec{
			ID:      "proc:" + p.ID + ":" + name,
			Name:    p.ID + " › " + name,
			Group:   "proc",
			Exe:     exe,
			Args:    tokens[1:],
			Env:     []string{"PATH=" + inst.Dir + string(os.PathListSeparator) + os.Getenv("PATH")},
			Dir:     p.Root,
			Probe:   &supervisor.AliveProbe{Grace: 2 * time.Second},
			Restart: defaultRestart(),
			LogPath: filepath.Join(logDir, "proc-"+p.ID+"-"+name+".log"),
		})
	}
	return specs
}

// splitArgs separa por espaços respeitando aspas simples e duplas (sem escapes).
// Suficiente para linhas de comando de manifesto; não é um shell.
func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	inTok := false
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
			inTok = true
		case r == ' ' || r == '\t':
			if inTok {
				out = append(out, cur.String())
				cur.Reset()
				inTok = false
			}
		default:
			cur.WriteRune(r)
			inTok = true
		}
	}
	if inTok {
		out = append(out, cur.String())
	}
	return out
}
