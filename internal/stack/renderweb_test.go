package stack

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hyphp/internal/paths"
	"hyphp/internal/state"
	"hyphp/internal/supervisor"
	"hyphp/internal/webserver"
)

// bindingWeb imita o nginx -t: a validação faz bind() de cada porta da
// config. É o que quebrava a troca Apache → nginx, com o Apache ainda
// segurando 80, 443 e a porta do phpMyAdmin.
type bindingWeb struct{}

func (bindingWeb) Name() state.WebServerName { return state.Nginx }

func (bindingWeb) Render(_ []webserver.Site, _ []webserver.PHPPool, ports webserver.Ports, _ string, tool *webserver.Tool) (map[string][]byte, error) {
	return map[string][]byte{"listen.txt": fmt.Appendf(nil, "%d %d %d", ports.HTTP, ports.HTTPS, tool.Port)}, nil
}

func (bindingWeb) Validate(etcDir string) error {
	raw, err := os.ReadFile(filepath.Join(etcDir, "listen.txt"))
	if err != nil {
		return err
	}
	for _, port := range strings.Fields(string(raw)) {
		ln, err := net.Listen("tcp4", "127.0.0.1:"+port)
		if err != nil {
			return err
		}
		ln.Close()
	}
	return nil
}

func (bindingWeb) Command(string) (string, []string, string) { return "", nil, "" }
func (bindingWeb) Probe(webserver.Ports) supervisor.Probe    { return supervisor.AliveProbe{} }
func (bindingWeb) PageDataDir() string                       { return "html/dados" }

func TestRenderWebValidaComAsPortasReaisOcupadas(t *testing.T) {
	t.Setenv(paths.EnvRoot, t.TempDir())
	held := make([]int, 3)
	for i := range held {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ln.Close() })
		held[i] = ln.Addr().(*net.TCPAddr).Port
	}
	st := state.State{HTTPPort: held[0], HTTPSPort: held[1]}
	out := desiredOutput{Tool: &webserver.Tool{Name: "phpmyadmin", Port: held[2]}}

	if _, err := (&Stack{}).renderWeb(bindingWeb{}, out, st); err != nil {
		t.Fatalf("validação falhou com as portas reais ocupadas pelo servidor atual: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(paths.Etc(), "nginx", "listen.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("%d %d %d", held[0], held[1], held[2]); string(got) != want {
		t.Fatalf("config promovida escuta em %q, quero as portas reais %q", got, want)
	}
	if out.Tool.Port != held[2] {
		t.Fatalf("renderWeb alterou a porta da ferramenta do chamador para %d", out.Tool.Port)
	}
}

// paginaWeb renderiza como o nginx: a página e a pasta dos dados com .keep.
type paginaWeb struct{}

func (paginaWeb) Name() state.WebServerName { return state.Nginx }
func (paginaWeb) Render([]webserver.Site, []webserver.PHPPool, webserver.Ports, string, *webserver.Tool) (map[string][]byte, error) {
	return map[string][]byte{"nginx.conf": []byte("conf\n"), "html/index.html": []byte("pagina\n"), "html/dados/.keep": nil}, nil
}
func (paginaWeb) Validate(string) error                     { return nil }
func (paginaWeb) Command(string) (string, []string, string) { return "", nil, "" }
func (paginaWeb) Probe(webserver.Ports) supervisor.Probe    { return supervisor.AliveProbe{} }
func (paginaWeb) PageDataDir() string                       { return "html/dados" }

// A lista de projetos muda a cada projeto e não é configuração: gravá-la não
// pode marcar o web server para reinício, e o render seguinte não pode
// apagá-la.
func TestListaDaPaginaNaoReiniciaOWebServer(t *testing.T) {
	t.Setenv(paths.EnvRoot, t.TempDir())
	s := &Stack{d: Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	st := state.State{HTTPPort: 80, HTTPSPort: 443}
	if _, err := s.renderWeb(paginaWeb{}, desiredOutput{}, st); err != nil {
		t.Fatal(err)
	}
	s.writePageData(paginaWeb{}, []webserver.Site{{ID: "app", Domain: "app.test"}}, st)
	changed, err := s.renderWeb(paginaWeb{}, desiredOutput{}, st)
	if err != nil || changed {
		t.Fatalf("render depois da lista: changed=%v err=%v", changed, err)
	}
	raw, err := os.ReadFile(filepath.Join(paths.Etc(), "nginx", "html", "dados", "projetos.json"))
	if err != nil || !strings.Contains(string(raw), `"app.test"`) {
		t.Fatalf("lista depois do render: %v\n%s", err, raw)
	}
}
