package services

import "testing"

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
