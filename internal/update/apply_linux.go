package update

import "hyphp/internal/i18n"

// updaterExe é a cópia do executável do app que aplicaria o update.
const updaterExe = "hyphp-updater"

// Launch no Linux ainda não existe: o update não é aplicado pelo app.
func Launch(ApplyRequest, string) error {
	return i18n.Errorf("err.linux.unavailable")
}

// apply no Linux: idem Launch.
func apply(ApplyRequest, func(string, ...any)) error {
	return i18n.Errorf("err.linux.unavailable")
}

// relaunch no Linux: idem Launch.
func relaunch(ApplyRequest) (string, error) {
	return "", i18n.Errorf("err.linux.unavailable")
}
