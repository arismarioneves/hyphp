// Package nginx implementa webserver.WebServer sobre o nginx para Windows,
// falando com os mesmos pools de php-cgi que o Apache usa (spec §6.7).
package nginx

import (
	"bytes"
	"embed"
	"fmt"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"text/template"

	hrender "hyphp/internal/render"
	"hyphp/internal/runtime"
	"hyphp/internal/state"
	"hyphp/internal/supervisor"
	"hyphp/internal/sysproc"
	"hyphp/internal/webserver"
)

//go:embed templates
var templatesFS embed.FS

var tmpl = template.Must(template.ParseFS(templatesFS, "templates/*.tmpl"))

type server struct{ inst runtime.Installed }

// New devolve o web server nginx descrito por inst. etcDir e logDir vêm por
// chamada (ver apache.New para o motivo).
func New(inst runtime.Installed) webserver.WebServer { return server{inst: inst} }

func (s server) Name() state.WebServerName { return state.Nginx }

type confData struct {
	ServerRoot  string
	LogDir      string
	Ports       webserver.Ports
	NamesBucket int
}

type upstreamsData struct {
	Pools []webserver.PHPPool
}

type toolData struct {
	Tool       webserver.Tool
	LogDir     string
	ServerRoot string
}

type siteData struct {
	Site       webserver.Site
	Ports      webserver.Ports
	LogDir     string
	ServerRoot string
}

func (s server) Render(sites []webserver.Site, pools []webserver.PHPPool, ports webserver.Ports, logDir string, tool *webserver.Tool) (map[string][]byte, error) {
	root := hrender.SlashDir(s.inst.Dir)
	log := hrender.SlashDir(logDir)

	files := make(map[string][]byte, len(sites)+4)

	main, err := render("nginx.conf.tmpl", confData{ServerRoot: root, LogDir: log, Ports: ports, NamesBucket: namesBucketSize(sites, tool)})
	if err != nil {
		return nil, err
	}
	files["nginx.conf"] = main

	ordered := slices.Clone(pools)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	ups, err := render("upstreams.conf.tmpl", upstreamsData{Pools: ordered})
	if err != nil {
		return nil, err
	}
	files["upstreams.conf"] = ups

	for _, site := range sites {
		if site.ID == "" {
			return nil, fmt.Errorf("nginx: site %q sem ID", site.Domain)
		}
		conf, err := render("site.conf.tmpl", siteData{Site: site, Ports: ports, LogDir: log, ServerRoot: root})
		if err != nil {
			return nil, fmt.Errorf("nginx: site %s: %w", site.ID, err)
		}
		key := "sites/" + site.ID + ".conf"
		if _, dup := files[key]; dup {
			return nil, fmt.Errorf("nginx: dois sites com o mesmo ID %q", site.ID)
		}
		files[key] = conf
	}

	if tool != nil {
		conf, err := render("tool.conf.tmpl", toolData{Tool: *tool, LogDir: log, ServerRoot: root})
		if err != nil {
			return nil, fmt.Errorf("nginx: ferramenta %s: %w", tool.Name, err)
		}
		files["sites/zz-tool-"+tool.Name+".conf"] = conf
	}

	files["html/index.html"] = webserver.DefaultIndexHTML()
	// logs/ e temp/ precisam existir dentro do prefixo -p: o nginx abre o log
	// de erro e os diretórios temporários antes de ler a config. O ".keep"
	// cria o diretório sem que render.WriteFiles passe a varrer o que o nginx
	// escreve lá dentro.
	// Mesmo motivo do apache: "include sites/*.conf" falha se o diretório não
	// existir, e sem projeto nenhum ele não existiria. Aqui é ".dir": o
	// conteúdo de sites/ é nosso e site de projeto removido tem de sumir.
	files["sites/"+hrender.DirFile] = nil
	files["logs/"+hrender.KeepFile] = nil
	files["temp/"+hrender.KeepFile] = nil
	return files, nil
}

// namesBucketSize devolve o server_names_hash_bucket_size em que cabe o nome
// mais longo. O padrão do nginx no Windows é 32 — a linha de cache que ele
// assume — e um domínio acima de ~22 caracteres já não cabe: o nginx -t
// recusa com "could not build server_names_hash". A conta é a do
// ngx_hash_init: ponteiro + nome + 2 alinhados, mais o ponteiro que fecha o
// bucket. Ponteiro de 8 bytes, para servir às builds de 32 e de 64 bits.
// Potência de 2 e mínimo 64, como a documentação do nginx sugere.
func namesBucketSize(sites []webserver.Site, tool *webserver.Tool) int {
	need := 0
	fit := func(name string) { need = max(need, (len(name)+2+7)&^7+16) }
	for _, site := range sites {
		fit(site.Domain)
		for _, alias := range site.Aliases {
			fit(alias)
		}
	}
	if tool != nil {
		fit(tool.Name + ".local")
	}
	size := 64
	for size < need {
		size *= 2
	}
	return size
}

// Validate roda `nginx -t -p <etcDir> -c nginx.conf` e exige
// "test is successful" na saída combinada (o nginx escreve em stderr).
func (s server) Validate(etcDir string) error {
	exe, args, dir := s.command(etcDir, true)
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	sysproc.Hide(cmd)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if !strings.Contains(text, "test is successful") {
		if err != nil {
			return fmt.Errorf("nginx: nginx -t recusou a config em %s: %w\n%s", etcDir, err, text)
		}
		return fmt.Errorf("nginx: nginx -t não reportou \"test is successful\" para %s:\n%s", etcDir, text)
	}
	return nil
}

func (s server) Command(etcDir string) (string, []string, string) {
	return s.command(etcDir, false)
}

func (s server) command(etcDir string, test bool) (string, []string, string) {
	args := make([]string, 0, 8)
	if test {
		args = append(args, "-t")
	}
	args = append(args, "-p", prefix(etcDir), "-c", "nginx.conf")
	if !test {
		// Sem isto o nginx.exe faz fork e some, e o supervisor perderia o
		// processo real.
		args = append(args, "-g", "daemon off;")
	}
	return s.inst.Exe, args, s.inst.Dir
}

func (s server) Probe(ports webserver.Ports) supervisor.Probe {
	return supervisor.HTTPProbe{URL: fmt.Sprintf("http://127.0.0.1:%d/", ports.HTTP)}
}

func render(name string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, fmt.Errorf("nginx: template %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// prefix é o valor de -p: caminho com "/" e COM barra final, como na receita
// do C16. O nginx concatena o prefixo com caminhos relativos.
func prefix(etcDir string) string {
	return hrender.SlashDir(etcDir) + "/"
}
