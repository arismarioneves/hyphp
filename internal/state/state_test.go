package state

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	d := Default()
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"SchemaVersion", d.SchemaVersion, 1},
		{"WebServer", d.WebServer, Apache},
		{"PoolSize", d.PoolSize, 4},
		{"HTTPPort", d.HTTPPort, 80},
		{"HTTPSPort", d.HTTPSPort, 443},
		{"MySQLPort", d.MySQLPort, 3306},
		{"MailpitSMTPPort", d.MailpitSMTPPort, 1025},
		{"MailpitHTTPPort", d.MailpitHTTPPort, 8025},
		{"DefaultPHP vazio (maior instalada)", d.DefaultPHP, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Fatalf("Default().%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
	if d.Roots == nil || d.PortAlloc == nil {
		t.Fatal("Roots e PortAlloc devem ser vazios, nao nil (serializam como [] e {})")
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		content *string // nil = arquivo ausente
		want    func() State
		wantErr bool
	}{
		{
			name:    "arquivo ausente devolve Default sem erro",
			content: nil,
			want:    Default,
		},
		{
			name:    "arquivo parcial preenche o resto com Default",
			content: new(`{"webServer":"nginx","roots":["C:\\DEV"]}`),
			want: func() State {
				s := Default()
				s.WebServer = Nginx
				s.Roots = []string{`C:\DEV`}
				return s
			},
		},
		{
			name:    "json invalido retorna erro",
			content: new(`{"webServer": `),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "var", "state.json")
			if tt.content != nil {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(*tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("esperava erro, veio nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if want := tt.want(); !reflect.DeepEqual(got, want) {
				t.Fatalf("Load() =\n%+v\nwant\n%+v", got, want)
			}
		})
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "var", "state.json") // var/ ainda nao existe: Save deve criar

	want := Default()
	want.WebServer = Nginx
	want.DefaultPHP = "8.1"
	want.PoolSize = 2
	want.Roots = []string{`C:\DEV`, `D:\src`}
	want.PortAlloc = map[string][]int{"php:8.1": {9000, 9001}, "php:7.2": {9002, 9003}}
	want.Editor = `C:\Users\x\AppData\Local\Programs\cursor\Cursor.exe`
	want.SidebarCollapsed = true

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("roundtrip divergiu:\n got %+v\nwant %+v", got, want)
	}
}

func TestSaveAtomico(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	first := Default()
	if err := Save(path, first); err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	second := Default()
	second.PoolSize = 8
	if err := Save(path, second); err != nil { // sobrescreve arquivo existente (rename sobre destino)
		t.Fatalf("Save 2: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("arquivo temporario sobrou: %s", e.Name())
		}
	}
	if len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatalf("esperava somente state.json, veio %v", entries)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.PoolSize != 8 {
		t.Fatalf("PoolSize = %d, want 8 (segundo Save nao substituiu)", got.PoolSize)
	}

	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "\n  \"schemaVersion\": 1") {
		t.Fatalf("esperava JSON indentado com schemaVersion, veio:\n%s", raw)
	}
}
