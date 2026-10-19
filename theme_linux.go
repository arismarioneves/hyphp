package main

import "hyphp/internal/desktop"

// systemDarkMode no Linux vem do portal: o WebKitGTK não o repassa ao
// prefers-color-scheme, então a interface também recebe o valor do Go.
func systemDarkMode() bool { return desktop.SystemDark() }
