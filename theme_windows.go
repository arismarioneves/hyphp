package main

import "github.com/wailsapp/wails/v3/pkg/w32"

// systemDarkMode lê o tema de apps do Windows no registro. app.Env.IsDarkMode
// não serve para a cor inicial da janela: antes de app.Run ele devolve false.
func systemDarkMode() bool { return w32.IsCurrentlyDarkMode() }
