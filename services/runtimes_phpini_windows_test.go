package services

import (
	"testing"

	"hyphp/internal/state"
)

// No Windows o php.ini aponta curl.cainfo e openssl.cafile para o bundle do
// HyPHP: a tela recusa as duas, como as demais gerenciadas, antes de
// perguntar ao PHP.
func TestSetIniSettingRecusaCAsNoWindows(t *testing.T) {
	for _, name := range []string{"curl.cainfo", "openssl.cafile"} {
		r, probes := iniService(t, fakeBuiltins)
		if err := r.SetIniSetting("8.3", name, "C:/meu.pem"); err == nil || *probes != 0 {
			t.Errorf("%s: err=%v probes=%d", name, err, *probes)
		}
	}
}

// Quem contornou o erro 60 do curl na 3.x pela tela tem curl.cainfo no
// state; o php.ini usa o bundle do HyPHP, então a tela não pode mostrá-la
// como diretiva do usuário.
func TestIniSettingsOmiteCAsGerenciadasNoWindows(t *testing.T) {
	r, _ := iniService(t, fakeBuiltins)
	if err := r.d.UpdateState(func(s *state.State) {
		s.PHPIni = map[string]map[string]string{"8.3": {"curl.cainfo": "C:/meu.pem"}}
	}); err != nil {
		t.Fatal(err)
	}
	list, err := r.IniSettings("8.3")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.Name == "curl.cainfo" {
			t.Errorf("curl.cainfo listada: %+v", s)
		}
	}
}
