package update

import "hyphp/internal/i18n"

// Launch ainda não existe no macOS: aplicar update é da M3.
func Launch(req ApplyRequest, updaterPath string) error {
	return i18n.Errorf("err.mac.unavailable")
}

// RunApply nunca roda no macOS (só o updater do Windows passa ApplyFlag);
// se rodar, encerra com falha em vez de fingir que aplicou.
func RunApply(args []string) int {
	return 1
}
