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
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/template"

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
}

type poolsData struct {
	Pools []webserver.PHPPool
}

type vhostData struct {
	Site   webserver.Site
	Ports  webserver.Ports
	LogDir string
}

func (s server) Render(sites []webserver.Site, pools []webserver.PHPPool, ports webserver.Ports, logDir string) (map[string][]byte, error) {
	root := slashDir(s.inst.Dir)
	log := slashDir(logDir)

	files := make(map[string][]byte, len(sites)+3)

	main, err := render("httpd.conf.tmpl", confData{ServerRoot: root, LogDir: log, Ports: ports})
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
	etc := slashDir(etcDir)
	args := make([]string, 0, 8)
	if test {
		args = append(args, "-t")
	}
	args = append(args,
		"-f", etc+"/httpd.conf",
		"-d", slashDir(s.inst.Dir),
		"-C", "Define "+etcVar+" "+etc,
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

// slashDir normaliza um diretório para "/" sem barra final. Apache aceita "/"
// no Windows e a barra final duplicaria separadores nos Include.
func slashDir(dir string) string {
	s := filepath.ToSlash(dir)
	if len(s) > 1 {
		s = strings.TrimSuffix(s, "/")
	}
	return s
}
