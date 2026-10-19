package main

// systemDarkMode no Mac assume o escuro, o tema padrão do app: a cor de fundo
// só vale até o WebView pintar, e a interface segue o prefers-color-scheme.
func systemDarkMode() bool { return true }
