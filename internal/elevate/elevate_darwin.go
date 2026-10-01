package elevate

import (
	"time"

	"hyphp/internal/i18n"
)

// No macOS a elevação (hosts, CA, DNS) só chega na M2. Até lá cada chamada
// falha com motivo claro: devolver nil faria a UI dar a ação por aplicada.

// RunElevated ainda não existe no macOS.
func RunElevated(exe string, args []string) error {
	return i18n.Errorf("err.mac.unavailable")
}

// HelperPath ainda não existe no macOS: o hyphp-helper é só do Windows.
func HelperPath() (string, error) {
	return "", i18n.Errorf("err.mac.unavailable")
}

// RunInstaller ainda não existe no macOS: a aplicação de update é da M3.
func RunInstaller(exe, params string, timeout time.Duration) (uint32, error) {
	return 0, i18n.Errorf("err.mac.unavailable")
}
