package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkdirs(t *testing.T, base string, rel ...string) string {
	t.Helper()
	p := filepath.Join(append([]string{base}, rel...)...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad(t *testing.T) {
	t.Run("sem yaml, com public", func(t *testing.T) {
		root := mkdirs(t, t.TempDir(), "App81")
		writeFile(t, filepath.Join(root, "public", "index.php"), "<?php")
		p, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		if p.HasManifest {
			t.Fatal("HasManifest deveria ser false")
		}
		if p.ID != "app81" || p.Name != "app81" || p.Domain != "app81.test" {
			t.Fatalf("defaults errados: %+v", p)
		}
		if p.Docroot != "public" || p.DocrootAbs != filepath.Join(root, "public") {
			t.Fatalf("docroot errado: %q / %q", p.Docroot, p.DocrootAbs)
		}
		if p.HasHtaccess {
			t.Fatal("HasHtaccess deveria ser false")
		}
	})

	t.Run("com yaml e .htaccess no docroot", func(t *testing.T) {
		root := mkdirs(t, t.TempDir(), "whatever")
		writeFile(t, filepath.Join(root, ManifestFile), "name: Acme App\nphp: \"7.2\"\ndocroot: www\nprocesses:\n  queue: php artisan queue:work\n")
		writeFile(t, filepath.Join(root, "www", ".htaccess"), "RewriteEngine On")
		p, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		if !p.HasManifest {
			t.Fatal("HasManifest deveria ser true")
		}
		if p.ID != "acme-app" || p.Name != "Acme App" || p.Domain != "acme-app.test" || p.PHP != "7.2" {
			t.Fatalf("campos errados: %+v", p)
		}
		if p.DocrootAbs != filepath.Join(root, "www") {
			t.Fatalf("DocrootAbs = %q", p.DocrootAbs)
		}
		if !p.HasHtaccess {
			t.Fatal("HasHtaccess deveria ser true (.htaccess em www/)")
		}
		if p.Processes["queue"] != "php artisan queue:work" {
			t.Fatalf("processes = %v", p.Processes)
		}
	})

	t.Run(".htaccess na raiz sem docroot", func(t *testing.T) {
		root := mkdirs(t, t.TempDir(), "legacy")
		writeFile(t, filepath.Join(root, "index.php"), "<?php")
		writeFile(t, filepath.Join(root, ".htaccess"), "")
		p, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		if p.Docroot != "" || p.DocrootAbs != root || !p.HasHtaccess {
			t.Fatalf("got %+v", p)
		}
	})

	t.Run("yaml inválido é erro", func(t *testing.T) {
		root := mkdirs(t, t.TempDir(), "bad")
		writeFile(t, filepath.Join(root, ManifestFile), "php: 8\n")
		if _, err := Load(root); err == nil {
			t.Fatal("esperava erro de validação para php: 8")
		}
	})

	t.Run("yaml malformado é erro", func(t *testing.T) {
		root := mkdirs(t, t.TempDir(), "broken")
		writeFile(t, filepath.Join(root, ManifestFile), "name: [\n")
		if _, err := Load(root); err == nil {
			t.Fatal("esperava erro de parse")
		}
	})
}

func TestDiscover(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "a-public", "public", "index.php"), "")
	writeFile(t, filepath.Join(base, "b-index", "index.php"), "")
	writeFile(t, filepath.Join(base, "c-composer", "composer.json"), "{}")
	writeFile(t, filepath.Join(base, "d-yaml", ManifestFile), "name: dee\n")
	mkdirs(t, base, "e-empty") // sem marcador → ignorado
	writeFile(t, filepath.Join(base, ".hidden", "index.php"), "")
	writeFile(t, filepath.Join(base, "node_modules", "index.php"), "")
	writeFile(t, filepath.Join(base, "vendor", "index.php"), "")
	writeFile(t, filepath.Join(base, "file.txt"), "") // arquivo, não dir

	projs, err := Discover([]string{base, filepath.Join(base, "nao-existe")})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range projs {
		ids = append(ids, p.ID)
	}
	want := "a-public,b-index,c-composer,dee"
	if got := strings.Join(ids, ","); got != want {
		t.Fatalf("ids = %s, want %s", got, want)
	}
	for _, p := range projs {
		if p.Root != filepath.Join(base, filepath.Base(p.Root)) {
			t.Fatalf("Root não é filho direto do root: %q", p.Root)
		}
	}
}

func TestDiscoverDuplicateID(t *testing.T) {
	r1, r2 := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(r1, "same", "index.php"), "")
	writeFile(t, filepath.Join(r2, "same", "index.php"), "")
	projs, err := Discover([]string{r1, r2})
	if err != nil {
		t.Fatal(err)
	}
	if len(projs) != 1 || projs[0].Root != filepath.Join(r1, "same") {
		t.Fatalf("esperava só o primeiro 'same' (de r1); got %+v", projs)
	}
}

func TestWriteRoundTrip(t *testing.T) {
	root := mkdirs(t, t.TempDir(), "app")
	writeFile(t, filepath.Join(root, "public", "index.php"), "")
	p, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	p.PHP = "8.1"
	p.Wildcard = true
	p.Processes = map[string]string{"queue": "php artisan queue:work --tries=3"}
	if err := p.Write(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(root, ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	// O contrato é haver um comentário no topo, não o texto dele: fixar a
	// frase transformava qualquer ajuste de redação em teste vermelho.
	if !strings.HasPrefix(s, "#") {
		t.Fatalf("faltou cabeçalho comentado:\n%s", s)
	}
	if !strings.Contains(s, "php: \"8.1\"") {
		t.Fatalf("php deve ser string entre aspas:\n%s", s)
	}
	// ordem das chaves segue a struct: name antes de domain antes de php
	if strings.Index(s, "name:") > strings.Index(s, "domain:") || strings.Index(s, "domain:") > strings.Index(s, "php:") {
		t.Fatalf("ordem das chaves fora da struct:\n%s", s)
	}

	again, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !again.HasManifest || again.PHP != "8.1" || !again.Wildcard || again.Processes["queue"] != "php artisan queue:work --tries=3" || again.Docroot != "public" {
		t.Fatalf("round-trip perdeu dados: %+v", again)
	}
}
