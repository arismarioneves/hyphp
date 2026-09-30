package services

import (
	"context"
	"path/filepath"
	"testing"

	"hyphp/internal/runtime"
	"hyphp/internal/state"
)

// iniService monta um RuntimesService com um PHP 8.3 fictício e as consultas ao
// php.exe trocadas por um mapa: knows é o que o "PHP" conhece e o builtin dele.
func iniService(t *testing.T, knows map[string]string) (*RuntimesService, *int) {
	t.Helper()
	r := NewRuntimesService(RuntimesDeps{
		BinDir:    t.TempDir(),
		State:     &state.State{},
		StatePath: filepath.Join(t.TempDir(), "state.json"),
	})
	r.installed = []runtime.Installed{{Kind: runtime.PHP, Version: "8.3.10", Major: "8.3", Dir: t.TempDir()}}
	r.scanned = true
	probes := 0
	r.iniKnows = func(_ context.Context, _ runtime.Installed, _ []string, name, _ string) (bool, error) {
		probes++
		_, ok := knows[name]
		return ok, nil
	}
	r.iniBuiltins = func(_ context.Context, _ runtime.Installed, _, names []string) (map[string]string, error) {
		out := map[string]string{}
		for _, n := range names {
			if v, ok := knows[n]; ok {
				out[n] = v
			}
		}
		return out, nil
	}
	return r, &probes
}

var fakeBuiltins = map[string]string{
	"memory_limit": "128M", "max_execution_time": "30", "max_input_time": "-1",
	"max_input_vars": "1000", "post_max_size": "8M", "upload_max_filesize": "2M",
	"display_errors": "1", "error_reporting": "", "date.timezone": "",
	"session.gc_maxlifetime": "1440",
}

// Nome fora do formato, diretiva gerenciada e valor vazio/multilinha são
// recusados antes de perguntar ao PHP; desconhecida é recusada pelo PHP. Nada
// disso pode chegar ao state.
func TestSetIniSettingRecusa(t *testing.T) {
	casos := []struct{ nome, name, value string }{
		{"ponto e vírgula", "a;b", "1"},
		{"maiúscula", "A", "1"},
		{"espaço", "max input", "1"},
		{"gerenciada", "extension_dir", "C:/x"},
		{"cgi.*", "cgi.fix_pathinfo", "0"},
		{"mail", "smtp_port", "25"},
		{"valor vazio", "max_input_vars", "  "},
		{"valor multilinha", "max_input_vars", "5000\nextension=evil"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			r, probes := iniService(t, fakeBuiltins)
			if err := r.SetIniSetting("8.3", c.name, c.value); err == nil {
				t.Fatal("aceitou")
			}
			if *probes != 0 {
				t.Error("perguntou ao PHP algo que devia recusar antes")
			}
			if len(r.d.State.PHPIni) != 0 {
				t.Errorf("state mudou: %v", r.d.State.PHPIni)
			}
		})
	}

	r, probes := iniService(t, fakeBuiltins)
	if err := r.SetIniSetting("8.3", "nao_existe", "1"); err == nil || *probes != 1 {
		t.Fatalf("desconhecida: err=%v probes=%d", err, *probes)
	}
	if err := r.SetIniSetting("7.4", "max_input_vars", "5000"); err == nil {
		t.Fatal("aceitou série não instalada")
	}
	if len(r.d.State.PHPIni) != 0 {
		t.Errorf("state mudou: %v", r.d.State.PHPIni)
	}
}

// Definir persiste, dispara o Reconcile e aparece como "user" com o padrão ao
// lado; restaurar volta à origem e tira a série do state quando esvazia.
func TestIniSettingsDefinirERestaurar(t *testing.T) {
	r, _ := iniService(t, fakeBuiltins)
	reconciles := 0
	r.d.OnChange = func([]runtime.Installed) { reconciles++ }

	for _, kv := range [][2]string{{"max_input_vars", " 5000 "}, {"memory_limit", "1G"}, {"session.gc_maxlifetime", "7200"}} {
		if err := r.SetIniSetting("8.3", kv[0], kv[1]); err != nil {
			t.Fatalf("SetIniSetting(%s): %v", kv[0], err)
		}
	}
	if reconciles != 3 {
		t.Errorf("reconciles = %d, quero 3", reconciles)
	}
	saved, err := state.Load(r.d.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.PHPIni["8.3"]["max_input_vars"] != "5000" {
		t.Errorf("state.json = %v", saved.PHPIni)
	}

	list, err := r.IniSettings("8.3")
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]IniSetting{}
	for _, s := range list {
		byName[s.Name] = s
	}
	want := map[string]IniSetting{
		"max_input_vars":         {"max_input_vars", "5000", "1000", "user"},
		"memory_limit":           {"memory_limit", "1G", "512M", "user"},
		"max_input_time":         {"max_input_time", "-1", "-1", "php"},
		"upload_max_filesize":    {"upload_max_filesize", "64M", "64M", "hyphp"},
		"opcache.enable":         {"opcache.enable", "1", "1", "hyphp"},
		"session.gc_maxlifetime": {"session.gc_maxlifetime", "7200", "1440", "user"},
	}
	for name, w := range want {
		if byName[name] != w {
			t.Errorf("%s = %+v, quero %+v", name, byName[name], w)
		}
	}
	if len(list) != len(curatedIni)+1 || list[len(list)-1].Name != "session.gc_maxlifetime" {
		t.Errorf("curadas primeiro e extras no fim: %v", list)
	}

	for _, n := range []string{"max_input_vars", "memory_limit", "session.gc_maxlifetime"} {
		if err := r.ResetIniSetting("8.3", n); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := r.d.State.PHPIni["8.3"]; ok {
		t.Errorf("série vazia ficou no state: %v", r.d.State.PHPIni)
	}
	list, _ = r.IniSettings("8.3")
	for _, s := range list {
		if s.Name == "memory_limit" && (s.Value != "512M" || s.Source != "hyphp") {
			t.Errorf("memory_limit depois de restaurar = %+v", s)
		}
		if s.Name == "session.gc_maxlifetime" {
			t.Error("extra restaurada continua listada")
		}
	}
}
