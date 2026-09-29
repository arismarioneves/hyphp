package i18n

import "golang.org/x/sys/windows"

// systemLanguages devolve os idiomas de interface do usuário no Windows, do
// preferido para o menos preferido ("pt-BR", "en-US").
func systemLanguages() []string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil {
		return nil
	}
	return langs
}
