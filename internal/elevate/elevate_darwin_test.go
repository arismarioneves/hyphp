package elevate

import (
	"testing"

	"hyphp/internal/i18n"
)

// Até a M2, pedir privilégio no Mac tem de falhar dizendo por quê; um nil
// faria a UI mostrar hosts/CA/DNS como aplicados.
func TestElevacaoIndisponivelNoMac(t *testing.T) {
	want := i18n.T("err.mac.unavailable")
	if err := RunElevated("/bin/true", nil); err == nil || err.Error() != want {
		t.Fatalf("RunElevated = %v", err)
	}
	if _, err := HelperPath(); err == nil || err.Error() != want {
		t.Fatalf("HelperPath = %v", err)
	}
}
