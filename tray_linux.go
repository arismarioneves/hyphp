package main

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"

	"hyphp/internal/desktop"
)

// trayHost diz se o painel mostra ícones de bandeja: o GNOME sem a extensão
// AppIndicator não mostra.
func trayHost() bool { return desktop.TrayHost() }

func watchTrayHost(ctx context.Context, fn func(present bool)) error {
	return desktop.WatchTrayHost(ctx, fn)
}

// prepareTray dá nome ao ícone, que sem rótulo o painel chama de "Wails". O
// clique no ícone não abre a janela: o Wails chama o mesmo handler quando o
// painel abre o menu, e a janela pularia na frente a cada menu aberto. Abrir o
// app fica com o primeiro item do menu.
func prepareTray(tray *application.SystemTray, _ func()) { tray.SetLabel("HyPHP") }
