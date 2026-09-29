//go:build !windows

package i18n

import "os"

// systemLanguages lê LANG ("pt_BR.UTF-8") fora do Windows.
func systemLanguages() []string {
	if l := os.Getenv("LANG"); l != "" {
		return []string{l}
	}
	return nil
}
