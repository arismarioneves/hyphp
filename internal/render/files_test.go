package render

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir string, files map[string][]byte) bool {
	t.Helper()
	changed, err := WriteFiles(dir, files)
	if err != nil {
		t.Fatalf("WriteFiles(%s) erro = %v", dir, err)
	}
	return changed
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler %s: %v", path, err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestWriteFilesCria(t *testing.T) {
	dir := t.TempDir()
	changed := write(t, dir, map[string][]byte{
		"httpd.conf":         []byte("ServerRoot\n"),
		"vhosts/app81.conf":  []byte("vhost 81\n"),
		"default/index.html": []byte("<html>\n"),
	})
	if !changed {
		t.Fatal("primeira escrita devia reportar changed=true")
	}
	if got := mustRead(t, filepath.Join(dir, "httpd.conf")); got != "ServerRoot\n" {
		t.Fatalf("httpd.conf = %q", got)
	}
	if got := mustRead(t, filepath.Join(dir, "vhosts", "app81.conf")); got != "vhost 81\n" {
		t.Fatalf("vhosts/app81.conf = %q", got)
	}
	if got := mustRead(t, filepath.Join(dir, "default", "index.html")); got != "<html>\n" {
		t.Fatalf("default/index.html = %q", got)
	}
}

func TestWriteFilesIdempotente(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{
		"httpd.conf":        []byte("ServerRoot\n"),
		"vhosts/app81.conf": []byte("vhost 81\n"),
	}
	write(t, dir, files)
	info, err := os.Stat(filepath.Join(dir, "httpd.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if changed := write(t, dir, files); changed {
		t.Fatal("segunda escrita idêntica devia reportar changed=false")
	}
	again, err := os.Stat(filepath.Join(dir, "httpd.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !again.ModTime().Equal(info.ModTime()) {
		t.Fatal("arquivo idêntico foi reescrito (mtime mudou)")
	}
}

func TestWriteFilesConteudoMudou(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string][]byte{"httpd.conf": []byte("Listen 80\n")})
	changed := write(t, dir, map[string][]byte{"httpd.conf": []byte("Listen 8080\n")})
	if !changed {
		t.Fatal("conteúdo diferente devia reportar changed=true")
	}
	if got := mustRead(t, filepath.Join(dir, "httpd.conf")); got != "Listen 8080\n" {
		t.Fatalf("httpd.conf = %q", got)
	}
}

func TestWriteFilesRemoveObsoletoEmSubdiretorio(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string][]byte{
		"httpd.conf":        []byte("x\n"),
		"vhosts/app81.conf": []byte("81\n"),
		"vhosts/app72.conf": []byte("72\n"),
	})

	changed := write(t, dir, map[string][]byte{
		"httpd.conf":        []byte("x\n"),
		"vhosts/app81.conf": []byte("81\n"),
	})
	if !changed {
		t.Fatal("remoção de vhost obsoleto devia reportar changed=true")
	}
	if exists(filepath.Join(dir, "vhosts", "app72.conf")) {
		t.Fatal("vhosts/app72.conf devia ter sido removido")
	}
	if !exists(filepath.Join(dir, "vhosts", "app81.conf")) {
		t.Fatal("vhosts/app81.conf não devia ter sido removido")
	}

	changed = write(t, dir, map[string][]byte{"httpd.conf": []byte("x\n")})
	if !changed {
		t.Fatal("esvaziar vhosts/ devia reportar changed=true")
	}
	if exists(filepath.Join(dir, "vhosts")) {
		t.Fatal("vhosts/ ficou vazio e devia ter sido removido")
	}
}

func TestWriteFilesRecusaCaminhoQueEscapa(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		key  string
	}{
		{"pai direto", "../fora.conf"},
		{"pai no meio", "vhosts/../../fora.conf"},
		{"absoluto posix", "/etc/passwd"},
		{"absoluto windows", `C:\Windows\System32\drivers\etc\hosts`},
		{"vazio", ""},
		{"ponto", "."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			changed, err := WriteFiles(dir, map[string][]byte{tc.key: []byte("x")})
			if err == nil {
				t.Fatalf("WriteFiles aceitou %q", tc.key)
			}
			if changed {
				t.Fatal("changed devia ser false quando a chave é recusada")
			}
		})
	}
}

func TestWriteFilesRemoveArquivoPreExistente(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "httpd.conf.bak")
	if err := os.WriteFile(stale, []byte("lixo"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed := write(t, dir, map[string][]byte{"httpd.conf": []byte("x\n")})
	if !changed {
		t.Fatal("changed devia ser true")
	}
	if exists(stale) {
		t.Fatal("arquivo pré-existente fora de files devia ter sido removido")
	}
}

func TestWriteFilesPreservaDiretorioComKeep(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{
		"nginx.conf": []byte("worker_processes 2;\n"),
		"logs/.keep": nil,
		"temp/.keep": nil,
	}
	write(t, dir, files)
	if !exists(filepath.Join(dir, "logs", ".keep")) || !exists(filepath.Join(dir, "temp", ".keep")) {
		t.Fatal(".keep devia ter sido criado em logs/ e temp/")
	}

	// O nginx cria isto sozinho dentro do prefixo; WriteFiles não pode apagar.
	if err := os.WriteFile(filepath.Join(dir, "logs", "error.log"), []byte("erro"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "temp", "client_body_temp"), 0o755); err != nil {
		t.Fatal(err)
	}

	if changed := write(t, dir, files); changed {
		t.Fatal("rerender idêntico devia reportar changed=false")
	}
	if !exists(filepath.Join(dir, "logs", "error.log")) {
		t.Fatal("logs/error.log foi apagado; .keep declara existência, não posse")
	}
	if !exists(filepath.Join(dir, "temp", "client_body_temp")) {
		t.Fatal("temp/client_body_temp foi apagado")
	}
	if !exists(filepath.Join(dir, "logs", ".keep")) {
		t.Fatal("logs/.keep sumiu")
	}
}
