// Package apache implementa webserver.WebServer sobre o Apache HTTP Server
// (build Apache Lounge para Windows), falando com os pools de php-cgi por
// mod_proxy_fcgi + mod_proxy_balancer. Nunca mod_php, nunca mod_fcgid
// (spec §6.1).
package apache

import (
	"bytes"
	"embed"
	"fmt"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"syscall"
	"text/template"

	hrender "hyphp/internal/render"
	"hyphp/internal/runtime"
	"hyphp/internal/state"
	"hyphp/internal/supervisor"
	"hyphp/internal/webserver"
)

//go:embed templates
var templatesFS embed.FS

var tmpl = template.Must(template.ParseFS(templatesFS, "templates/*.tmpl"))

// etcVar é o nome da variável que carrega o diretório de configuração. Ela
// existe para que nenhum arquivo gerado precise conhecer o caminho — ver a
// nota de relocabilidade em webserver.WebServer.
const etcVar = "HYPHP_ETC"

type server struct{ inst runtime.Installed }

// New devolve o web server Apache descrito por inst. etcDir e logDir não entram
// aqui: vêm por chamada, porque o stack renderiza e valida em etc/apache.next
// antes de promover para etc/apache.
func New(inst runtime.Installed) webserver.WebServer { return server{inst: inst} }

func (s server) Name() state.WebServerName { return state.Apache }

type confData struct {
	ServerRoot string
	LogDir     string
	Ports      webserver.Ports
	Tool       *webserver.Tool // nil quando nenhuma ferramenta está instalada
}

type toolData struct {
	Tool   webserver.Tool
	LogDir string
}

type poolsData struct {
	Pools []webserver.PHPPool
}

type vhostData struct {
	Site   webserver.Site
	Ports  webserver.Ports
	LogDir string
}

func (s server) Render(sites []webserver.Site, pools []webserver.PHPPool, ports webserver.Ports, logDir string, tool *webserver.Tool) (map[string][]byte, error) {
	root := hrender.SlashDir(s.inst.Dir)
	log := hrender.SlashDir(logDir)

	files := make(map[string][]byte, len(sites)+3)

	main, err := render("httpd.conf.tmpl", confData{ServerRoot: root, LogDir: log, Ports: ports, Tool: tool})
	if err != nil {
		return nil, err
	}
	files["httpd.conf"] = main

	ordered := slices.Clone(pools)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	poolsConf, err := render("pools.conf.tmpl", poolsData{Pools: ordered})
	if err != nil {
		return nil, err
	}
	files["pools.conf"] = poolsConf

	for _, site := range sites {
		if site.ID == "" {
			return nil, fmt.Errorf("apache: site %q sem ID", site.Domain)
		}
		conf, err := render("vhost.conf.tmpl", vhostData{Site: site, Ports: ports, LogDir: log})
		if err != nil {
			return nil, fmt.Errorf("apache: vhost %s: %w", site.ID, err)
		}
		key := "vhosts/" + site.ID + ".conf"
		if _, dup := files[key]; dup {
			return nil, fmt.Errorf("apache: dois sites com o mesmo ID %q", site.ID)
		}
		files[key] = conf
	}

	if tool != nil {
		conf, err := render("tool.conf.tmpl", toolData{Tool: *tool, LogDir: log})
		if err != nil {
			return nil, fmt.Errorf("apache: ferramenta %s: %w", tool.Name, err)
		}
		files["tools/"+tool.Name+".conf"] = conf
	}

	// IncludeOptional cobre ARQUIVO ausente, não DIRETÓRIO ausente: sem nenhum
	// projeto a pasta vhosts/ não existiria e o httpd -t recusaria a config
	// inteira com "Could not open directory". Isso derrubava o Reconcile no
	// boot e nenhum serviço chegava a ser criado — nem MySQL, nem Mailpit, nem
	// o próprio Apache. O ".dir" garante o diretório sem gerar config e, ao
	// contrário do ".keep", deixa WriteFiles apagar vhosts de projetos que
	// saíram da lista.
	files["vhosts/"+hrender.DirFile] = nil
	files["tools/"+hrender.DirFile] = nil

	files["default/index.html"] = webserver.DefaultIndexHTML()
	return files, nil
}

// Validate roda `httpd -t` contra a config em etcDir. Exige "Syntax OK" na
// saída combinada: o Apache escreve tanto o diagnóstico quanto o "Syntax OK"
// em stderr, e um exit code 0 sozinho não prova nada.
func (s server) Validate(etcDir string) error {
	exe, args, dir := s.command(etcDir, true)
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if !strings.Contains(text, "Syntax OK") {
		if err != nil {
			return fmt.Errorf("apache: httpd -t recusou a config em %s: %w\n%s", etcDir, err, text)
		}
		return fmt.Errorf("apache: httpd -t não reportou Syntax OK para %s:\n%s", etcDir, text)
	}
	return nil
}

func (s server) Command(etcDir string) (string, []string, string) {
	return s.command(etcDir, false)
}

func (s server) command(etcDir string, test bool) (string, []string, string) {
	etc := hrender.SlashDir(etcDir)
	args := make([]string, 0, 8)
	if test {
		args = append(args, "-t")
	}
	// -C é tokenizado como uma linha de config: sem aspas, um perfil com
	// espaço ("C:/Users/João Silva/...") daria três argumentos ao Define.
	args = append(args,
		"-f", etc+"/httpd.conf",
		"-d", hrender.SlashDir(s.inst.Dir),
		"-C", "Define "+etcVar+" \""+etc+"\"",
	)
	return s.inst.Exe, args, s.inst.Dir
}

func (s server) Probe(ports webserver.Ports) supervisor.Probe {
	return supervisor.HTTPProbe{URL: fmt.Sprintf("http://127.0.0.1:%d/", ports.HTTP)}
}

func render(name string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, fmt.Errorf("apache: template %s: %w", name, err)
	}
	return buf.Bytes(), nil
}
