package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"acme", "acme"},
		{"My App", "my-app"},
		{"Acme_App v2", "acme-app-v2"},
		{"--weird--", "weird"},
		{"ÁÇÃO", "o"}, // não-ASCII vira "-" e é colapsado; sobra o que é [a-z0-9]
		{"", ""},
		{"___", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := NormalizeName(c.in); got != c.want {
				t.Fatalf("NormalizeName(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	withPublic := filepath.Join(t.TempDir(), "My App")
	if err := os.MkdirAll(filepath.Join(withPublic, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	noPublic := filepath.Join(t.TempDir(), "legacy")
	if err := os.MkdirAll(noPublic, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		in   Manifest
		root string
		want Manifest
	}{
		{
			name: "vazio com public",
			in:   Manifest{},
			root: withPublic,
			want: Manifest{Name: "my-app", Domain: "my-app.test", Docroot: "public"},
		},
		{
			name: "vazio sem public",
			in:   Manifest{},
			root: noPublic,
			want: Manifest{Name: "legacy", Domain: "legacy.test", Docroot: ""},
		},
		{
			name: "mantém valores explícitos",
			in:   Manifest{Name: "custom", Domain: "outro.test", Docroot: "www", PHP: "7.2"},
			root: withPublic,
			want: Manifest{Name: "custom", Domain: "outro.test", Docroot: "www", PHP: "7.2"},
		},
		{
			name: "domínio derivado do nome normalizado",
			in:   Manifest{Name: "Acme App"},
			root: noPublic,
			want: Manifest{Name: "Acme App", Domain: "acme-app.test"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := c.in
			ApplyDefaults(&m, c.root)
			if m.Name != c.want.Name || m.Domain != c.want.Domain || m.Docroot != c.want.Docroot || m.PHP != c.want.PHP {
				t.Fatalf("got %+v, want %+v", m, c.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		in      Manifest
		wantErr bool
	}{
		{"ok mínimo", Manifest{Name: "a", Domain: "a.test"}, false},
		{"ok completo", Manifest{Name: "a", Domain: "a.test", PHP: "8.1", Processes: map[string]string{"queue": "php artisan queue:work"}}, false},
		{"nome vazio", Manifest{Domain: "a.test"}, true},
		{"nome sem caracteres válidos", Manifest{Name: "___", Domain: "a.test"}, true},
		{"domínio sem .test", Manifest{Name: "a", Domain: "a.local"}, true},
		{"domínio vazio", Manifest{Name: "a"}, true},
		{"php só major", Manifest{Name: "a", Domain: "a.test", PHP: "8"}, true},
		{"php com patch", Manifest{Name: "a", Domain: "a.test", PHP: "8.1.10"}, true},
		{"process com chave vazia", Manifest{Name: "a", Domain: "a.test", Processes: map[string]string{"": "php x"}}, true},
		{"process com comando vazio", Manifest{Name: "a", Domain: "a.test", Processes: map[string]string{"queue": "   "}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("Validate(%+v) err = %v, wantErr %v", c.in, err, c.wantErr)
			}
		})
	}
}
