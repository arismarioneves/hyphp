package elevate

import "hyphp/internal/i18n"

// RunElevated no Linux ainda não existe: nenhuma ação do HyPHP pede senha
// aqui, e cada uma devolve este erro em vez de fingir que gravou.
func RunElevated(exe string, args []string) error {
	return i18n.Errorf("err.linux.unavailable")
}

// HelperPath no Linux: idem RunElevated.
func HelperPath() (string, error) {
	return "", i18n.Errorf("err.linux.unavailable")
}
