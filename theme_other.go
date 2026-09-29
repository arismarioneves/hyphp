//go:build !windows

package main

// systemDarkMode fora do Windows assume o escuro, o tema padrão do app.
func systemDarkMode() bool { return true }
