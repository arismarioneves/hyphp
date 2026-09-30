package stack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"hyphp/internal/compat"
	"hyphp/internal/i18n"
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
	Tool       *webserver.Tool     // phpMyAdmin quando instalado e com PHP compatível
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
		major := ResolvePHPMajor(p.PHP, in.State.DefaultPHP, phps)
		inst, ok := runtime.PHPByMajor(phps, major)
		if !ok {
			out.Warnings = append(out.Warnings, Warning{
				Code:      "php-missing",
				ProjectID: p.ID,
				Message:   i18n.T("warn.phpMissing", major, p.ID),
			})
			continue
		}
		srv = append(srv, served{p: p, inst: inst})
		byMajor[inst.Major] = append(byMajor[inst.Major], p)
	}

	// 2. pools: majors com projeto, mais a série que o phpMyAdmin precisa.
	//
	// Sem esse acréscimo a ferramenta ficava inutilizável em ambiente sem
	// projeto: os pools nasciam só dos projetos, o phpMyAdmin não achava
	// nenhuma série para rodar e o aviso saía com a lista vazia
	// ("instaladas: "), acusando ausência de PHP com o PHP instalado.
	if major := phpParaFerramenta(in, phps); major != "" && byMajor[major] == nil {
		// Slice vazia, nunca nil: o laço abaixo libera ranges cujo major virou
		// órfão comparando com nil, e um nil aqui faria o pool ser liberado logo
		// depois de criado.
		byMajor[major] = []project.Project{}
	}
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

	// 5. banco e e-mail (plano 07) — antes dos procs, que dependem deles
	if sp, ok := mysqlSpec(in); ok {
		out.Specs = append(out.Specs, sp)
	}
	if sp, ok := mailpitSpec(in); ok {
		out.Specs = append(out.Specs, sp)
	}

	// 6. processes dos projetos
	for _, s := range srv {
		specs, warns := procSpecs(s.p, s.inst, in.LogDir)
		out.Specs = append(out.Specs, specs...)
		out.Warnings = append(out.Warnings, warns...)
	}

	// 7. ferramentas servidas pelo web server (phpMyAdmin)
	tool, toolWarns := toolPhpMyAdmin(in, out.Pools)
	out.Tool = tool
	out.Warnings = append(out.Warnings, toolWarns...)

	return out, nil
}

// ResolvePHPMajor aplica a precedência de série do PHP: manifesto do projeto,
// depois state.DefaultPHP, depois a maior instalada.
//
// Exportada porque a UI precisa mostrar a série que o projeto vai usar de fato.
// Sem isto o frontend reimplementava os dois primeiros passos e parava antes do
// terceiro, exibindo "PHP —" para todo projeto sem `php:` no manifesto quando
// DefaultPHP está vazio — que é o caso padrão.
func ResolvePHPMajor(doManifesto, doState string, phps []runtime.Installed) string {
	if doManifesto != "" {
		return doManifesto
	}
	if doState != "" {
		return doState
	}
	return HighestPHPMajor(phps)
}

// HighestPHPMajor devolve o maior "major.minor" instalado ("" se nenhum).
// Comparação numérica por componente, não lexicográfica (8.10 > 8.9).
//
// Exportada porque a regra "state.DefaultPHP vazio = maior instalada" também
// vale fora do Reconcile (AppService.AddDefaultPHPToUserPath); duplicá-la faria
// o PATH do usuário divergir da versão que a stack realmente serve.
func HighestPHPMajor(phps []runtime.Installed) string {
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

// unionSorted junta as extensões globais (state.PHPExtensions[major]) com as
// declaradas nos manifestos dos projetos daquele major.
//
// Quando NADA foi configurado — nem no state, nem em manifesto algum — vale
// runtime.DefaultExtensions (C18.4). Sem isso o php.ini sai sem uma única
// linha `extension=`, e um `new PDO('mysql:...')` falha com "could not find
// driver": um ambiente PHP sem pdo_mysql, curl, intl nem gd não serve para
// nada, e o usuário não tem como adivinhar que precisa declarar cada uma.
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
	if len(set) == 0 {
		return slices.Clone(runtime.DefaultExtensions)
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
				Message: i18n.T("warn.tlsSite", p.Domain, err),
			})
		} else {
			site.TLSCert = filepath.ToSlash(cert)
			site.TLSKey = filepath.ToSlash(key)
		}
	}
	if in.State.WebServer == state.Nginx && p.HasHtaccess {
		warns = append(warns, Warning{
			Code: "htaccess-under-nginx", ProjectID: p.ID,
			Message: i18n.T("warn.htaccessUnderNginx", p.ID),
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

// procSpecs gera um spec por entrada de processes (C15): Dir = Root, PATH com a
// pasta do PHP do projeto na frente e executável resolvido contra esse PATH.
func procSpecs(p project.Project, inst runtime.Installed, logDir string) ([]supervisor.Spec, []Warning) {
	names := make([]string, 0, len(p.Processes))
	for n := range p.Processes {
		names = append(names, n)
	}
	sort.Strings(names)

	// A pasta do PHP vai na frente para `php` e `composer` do projeto vencerem
	// qualquer PHP que exista no PATH da máquina.
	pathEnv := inst.Dir + string(os.PathListSeparator) + os.Getenv("PATH")
	specs := make([]supervisor.Spec, 0, len(names))
	var warns []Warning
	for _, name := range names {
		tokens := splitCommand(p.Processes[name])
		if len(tokens) == 0 {
			continue // Validate já impede; defesa contra manifesto editado à mão
		}
		exe, args, err := resolveProcExe(tokens, inst, pathEnv)
		if err != nil {
			warns = append(warns, Warning{
				Code: "proc-exe-missing", ProjectID: p.ID,
				Message: fmt.Sprintf("%s › %s: %v", p.ID, name, err),
			})
			continue
		}
		specs = append(specs, supervisor.Spec{
			ID:      "proc:" + p.ID + ":" + name,
			Name:    p.ID + " › " + name,
			Group:   "proc",
			Exe:     exe,
			Args:    args,
			Env:     []string{"PATH=" + pathEnv},
			Dir:     p.Root,
			Probe:   &supervisor.AliveProbe{Grace: 2 * time.Second},
			Restart: defaultRestart(),
			LogPath: filepath.Join(logDir, "proc-"+p.ID+"-"+name+".log"),
		})
	}
	return specs, warns
}

// resolveProcExe transforma os tokens da linha de comando em (Exe, Args).
//
// Resolver aqui, e não deixar para o exec, é obrigatório: os/exec faz LookPath
// com o PATH do processo pai e ignora cmd.Env, então um `npm` ou `composer` do
// manifesto nunca enxergaria o PATH montado para o projeto.
func resolveProcExe(tokens []string, inst runtime.Installed, pathEnv string) (string, []string, error) {
	head := strings.TrimSuffix(strings.ToLower(tokens[0]), ".exe")
	rest := tokens[1:]
	switch head {
	case "php":
		return inst.Exe, rest, nil
	case "composer":
		if exe, err := lookPathIn(pathEnv, "composer"); err == nil {
			return exe, rest, nil
		}
		// Instalação que só copiou o .phar: roda com o PHP do projeto.
		phar, err := findInPath(pathEnv, "composer.phar")
		if err != nil {
			return "", nil, errors.New(i18n.T("err.stack.composerNotFound"))
		}
		return inst.Exe, append([]string{phar}, rest...), nil
	default:
		exe, err := lookPathIn(pathEnv, tokens[0])
		if err != nil {
			return "", nil, errors.New(i18n.T("err.stack.exeNotFound", tokens[0]))
		}
		return exe, rest, nil
	}
}

// lookPathIn é substituída nos testes por uma resolução determinística.
var lookPathIn = defaultLookPathIn

// defaultLookPathIn procura file nos diretórios de pathEnv, aplicando as
// extensões de PATHEXT quando file não tem extensão. Um caminho com separador
// é usado como veio, só confirmando que existe.
func defaultLookPathIn(pathEnv, file string) (string, error) {
	if strings.ContainsAny(file, `\/`) {
		if isExecFile(file) {
			return file, nil
		}
		return "", fmt.Errorf("stack: %s não existe", file)
	}
	exts := []string{""}
	if filepath.Ext(file) == "" {
		// PATHEXT vem em maiúsculas (".COM;.EXE;.BAT"). O sistema de arquivos do
		// Windows é case-insensitive, mas o caminho vai para o Spec, aparece na
		// UI e entra na comparação do Reconcile: minúsculas mantêm o valor
		// estável e legível.
		for _, e := range strings.Split(pathExt(), ";") {
			if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
				exts = append(exts, e)
			}
		}
	}
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		for _, ext := range exts {
			cand := filepath.Join(dir, file+ext)
			if isExecFile(cand) {
				return cand, nil
			}
		}
	}
	return "", fmt.Errorf("stack: %s não encontrado no PATH", file)
}

// findInPath procura um arquivo exato (sem PATHEXT) nos diretórios de pathEnv.
func findInPath(pathEnv, file string) (string, error) {
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		cand := filepath.Join(dir, file)
		if isExecFile(cand) {
			return cand, nil
		}
	}
	return "", fmt.Errorf("stack: %s não encontrado no PATH", file)
}

func pathExt() string {
	if v := os.Getenv("PATHEXT"); v != "" {
		return v
	}
	return ".COM;.EXE;.BAT;.CMD"
}

func isExecFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// splitCommand separa a linha de comando de `processes` em tokens.
//
// Regras: as do CommandLineToArgvW (que é o que o Windows usa para desfazer o
// syscall.EscapeArg do os/exec), mais aspas simples por conveniência de
// manifesto. Espaços e tabs separam; "..." e '...' agrupam; dentro de "...",
// \" é uma aspa literal e 2n barras invertidas antes de uma aspa viram n
// barras. Barra invertida que NÃO precede aspas é literal — sem essa regra
// C:\temp\arquivo viraria C:temparquivo, e manifesto Windows é cheio deles.
func splitCommand(s string) []string {
	var (
		out   []string
		cur   strings.Builder
		inTok bool
		quote rune
		bs    int // barras invertidas pendentes
	)
	flushBS := func() {
		if bs > 0 {
			cur.WriteString(strings.Repeat(`\`, bs))
			bs = 0
		}
	}
	for _, r := range s {
		switch {
		case r == '\\' && quote != '\'':
			bs++
			inTok = true
		case r == '"' && quote != '\'':
			cur.WriteString(strings.Repeat(`\`, bs/2))
			odd := bs%2 == 1
			bs = 0
			if odd {
				cur.WriteRune('"')
			} else if quote == '"' {
				quote = 0
			} else {
				quote = '"'
			}
			inTok = true
		case r == '\'' && quote != '"':
			flushBS()
			if quote == '\'' {
				quote = 0
			} else {
				quote = '\''
			}
			inTok = true
		case (r == ' ' || r == '\t') && quote == 0:
			flushBS()
			if inTok {
				out = append(out, cur.String())
				cur.Reset()
				inTok = false
			}
		default:
			flushBS()
			cur.WriteRune(r)
			inTok = true
		}
	}
	flushBS()
	if inTok {
		out = append(out, cur.String())
	}
	return out
}

// MySQLSpecID e MailpitSpecID são os IDs fixos do contrato (C3/C11).
const (
	MySQLSpecID   = "mysql"
	MailpitSpecID = "mailpit"
)

// mysqlSpec devolve o spec do servidor de banco — MySQL ou MariaDB, pelo
// DBRuntime —, ou ok=false quando não há o que subir. O ID continua "mysql"
// nos dois motores: é o serviço de banco da stack, e a UI, o Banco e o
// phpMyAdmin o procuram por esse nome. inst.Exe já é o servidor do motor
// (mysqld.exe ou mariadbd.exe).
func mysqlSpec(in desiredInput) (supervisor.Spec, bool) {
	inst, ok := DBRuntime(in.Runtimes, in.State)
	if !ok {
		return supervisor.Spec{}, false
	}
	logName := "mysql.log"
	if inst.Kind == runtime.MariaDB {
		logName = "mariadb.log"
	}
	addr := fmt.Sprintf("127.0.0.1:%d", in.State.MySQLPort)
	return supervisor.Spec{
		ID:    MySQLSpecID,
		Name:  DBName(inst.Kind) + " " + inst.Version,
		Group: "db",
		Exe:   inst.Exe,
		Args: []string{
			"--defaults-file=" + MyIniPath(in.EtcDir),
			"--console",
		},
		Dir:          inst.Dir,
		Port:         in.State.MySQLPort,
		Probe:        &supervisor.MySQLProbe{Addr: addr, Version: inst.Version},
		ProbeTimeout: 60 * time.Second,
		Restart:      defaultRestart(),
		LogPath:      filepath.Join(in.LogDir, logName),
	}, true
}

// mailpitSpec devolve o spec do Mailpit, ou ok=false quando não há Mailpit em
// bin/. Flags conferidas no cmd/root.go do Mailpit v1.31.1: --smtp, --listen e
// --database. O binário fica direto em bin/mailpit/mailpit.exe (C18.5), então
// inst.Exe já é o caminho final.
func mailpitSpec(in desiredInput) (supervisor.Spec, bool) {
	inst, ok := runtime.Newest(in.Runtimes, runtime.Mailpit)
	if !ok {
		return supervisor.Spec{}, false
	}
	httpAddr := fmt.Sprintf("127.0.0.1:%d", in.State.MailpitHTTPPort)
	return supervisor.Spec{
		ID:    MailpitSpecID,
		Name:  "Mailpit " + inst.Version,
		Group: "mail",
		Exe:   inst.Exe,
		Args: []string{
			"--smtp", fmt.Sprintf("127.0.0.1:%d", in.State.MailpitSMTPPort),
			"--listen", httpAddr,
			"--database", filepath.Join(in.VarDir, "mailpit.db"),
		},
		Dir:          inst.Dir,
		Port:         in.State.MailpitHTTPPort,
		Probe:        &supervisor.TCPProbe{Addr: httpAddr},
		ProbeTimeout: 20 * time.Second,
		Restart:      defaultRestart(),
		LogPath:      filepath.Join(in.LogDir, "mailpit.log"),
	}, true
}

// toolPhpMyAdmin monta o vhost do phpMyAdmin quando ele está instalado e
// existe uma série de PHP compatível.
//
// A escolha da série não pode ser a padrão do projeto: o phpMyAdmin 5.2 não
// roda em PHP 8.3+, e quem tem 8.4 como padrão veria a ferramenta abrir numa
// tela de erro de sintaxe. Aqui ele é servido pela maior série instalada dentro
// da faixa, ainda que nenhum projeto use essa série — o pool já existe porque
// todo PHP instalado ganha o seu.
func toolPhpMyAdmin(in desiredInput, pools []webserver.PHPPool) (*webserver.Tool, []Warning) {
	pma, ok := runtime.Newest(in.Runtimes, runtime.PhpMyAdmin)
	if !ok {
		return nil, nil
	}

	faixa, ok := compat.PHPParaPhpMyAdmin(pma.Version)
	if !ok {
		// Versão fora da tabela: servir assim mesmo é melhor do que recusar
		// uma combinação que pode funcionar. O aviso registra a incerteza.
		faixa = compat.Faixa{}
	}

	// A lista vem dos PHP INSTALADOS, não dos pools: pool só existe quando há
	// projeto usando a série, e a mensagem precisa dizer o que está instalado
	// na máquina. Com pools, ela saía como "(instaladas: )" — acusando
	// ausência de PHP num ambiente com PHP instalado.
	instaladas := make([]string, 0, len(in.Runtimes))
	for _, p := range runtime.ByKind(in.Runtimes, runtime.PHP) {
		instaladas = append(instaladas, p.Major)
	}
	sort.Strings(instaladas)
	instaladas = slices.Compact(instaladas)

	escolhida := majorCompativel(runtime.ByKind(in.Runtimes, runtime.PHP), faixa)
	if escolhida == "" {
		tem := i18n.T("warn.pmaSemPhpNone")
		if len(instaladas) > 0 {
			tem = i18n.T("warn.pmaSemPhpInstalled", strings.Join(instaladas, ", "))
		}
		return nil, []Warning{{
			Code:    "pma-sem-php",
			Message: i18n.T("warn.pmaSemPhp", pma.Version, faixa, tem),
		}}
	}
	return &webserver.Tool{
		Name:     "phpmyadmin",
		Port:     in.State.PhpMyAdminPort,
		Docroot:  filepath.ToSlash(pma.Dir),
		PoolName: webserver.PoolName(escolhida),
	}, nil
}

// phpParaFerramenta devolve a série de PHP que o phpMyAdmin instalado exige,
// ou "" quando ele não está instalado ou nenhuma série serve.
//
// Olha os PHP INSTALADOS, não os pools: é justamente o caso em que ainda não
// há pool nenhum — ambiente sem projeto — que precisa criar um.
func phpParaFerramenta(in desiredInput, phps []runtime.Installed) string {
	pma, ok := runtime.Newest(in.Runtimes, runtime.PhpMyAdmin)
	if !ok {
		return ""
	}
	faixa, ok := compat.PHPParaPhpMyAdmin(pma.Version)
	if !ok {
		faixa = compat.Faixa{}
	}
	return majorCompativel(phps, faixa)
}

// majorCompativel devolve o major do PHP instalado mais novo que cabe na faixa.
//
// O teste usa a versão COMPLETA e o resultado é o major, porque o pool é por
// série. Comparar o major contra a faixa rejeitava indevidamente: o piso do
// phpMyAdmin 5.2 é 7.2.5, e o major "7.2" é menor que isso — o PHP 7.2.34
// instalado, que atende de sobra, era descartado.
func majorCompativel(phps []runtime.Installed, f compat.Faixa) string {
	versoes := make([]string, 0, len(phps))
	porVersao := make(map[string]string, len(phps))
	for _, p := range phps {
		versoes = append(versoes, p.Version)
		porVersao[p.Version] = p.Major
	}
	return porVersao[compat.MelhorPHP(versoes, f)]
}
