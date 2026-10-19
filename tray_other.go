//go:build !linux

package main

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// trayHost: a área de notificação do Windows e a barra de menus do Mac sempre
// mostram o ícone.
func trayHost() bool { return true }

func watchTrayHost(context.Context, func(present bool)) error { return nil }

// prepareTray faz o clique esquerdo abrir o app, como em todo ícone de bandeja
// do Windows; o padrão do Wails é não fazer nada, e só o menu do clique
// direito abriria.
func prepareTray(tray *application.SystemTray, open func()) { tray.OnClick(open) }
