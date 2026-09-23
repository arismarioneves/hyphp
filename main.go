package main

import (
	"embed"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"hyphp/internal/paths"
	"hyphp/internal/state"
	"hyphp/internal/supervisor"
	"hyphp/services"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/tray.png
var trayIcon []byte

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := paths.EnsureLayout(); err != nil {
		logger.Error("criar layout de runtime", "root", paths.Root(), "err", err)
		os.Exit(1)
	}
	statePath := filepath.Join(paths.Var(), "state.json")
	st, err := state.Load(statePath)
	if err != nil {
		logger.Error("carregar state.json", "path", statePath, "err", err)
		os.Exit(1)
	}
	// Persiste os defaults na primeira execução (idempotente nas seguintes).
	if err := state.Save(statePath, st); err != nil {
		logger.Error("gravar state.json", "path", statePath, "err", err)
		os.Exit(1)
	}
	logger.Info("hyphp iniciando", "root", paths.Root(), "webServer", st.WebServer)

	// O supervisor nasce antes da UI: New() associa o processo ao Job Object
	// kill-on-close, e todo filho criado a partir daqui herda esse job.
	sup, err := supervisor.New(logger)
	if err != nil {
		logger.Error("criar supervisor", "err", err)
		os.Exit(1)
	}
	// stopEvents é preenchido depois de application.New e usado no OnShutdown.
	var stopEvents func()

	// quitting distingue "usuário fechou a janela" (ocultar) de "Sair" (encerrar):
	// o hook WindowClosing só cancela o fechamento enquanto quitting == false.
	var quitting atomic.Bool
	var window *application.WebviewWindow
	var tray *application.SystemTray

	showMain := func() {
		if window != nil {
			window.Show()
			window.Focus()
		}
	}

	app := application.New(application.Options{
		Name:        "HyPHP",
		Description: "Ambiente de desenvolvimento PHP",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.hyphp",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				logger.Info("segunda instancia ignorada", "args", data.Args)
				showMain()
			},
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		OnShutdown: func() {
			// hyphp: shutdown
			if stopEvents != nil {
				stopEvents()
			}
			if err := sup.Close(); err != nil {
				logger.Error("encerrar supervisor", "err", err)
			}
			if tray != nil {
				tray.Destroy()
			}
		},
	})

	quit := func() {
		quitting.Store(true)
		app.Quit()
	}

	// hyphp: services
	app.RegisterService(application.NewService(services.NewAppService(services.AppDeps{
		Quit:      quit,
		State:     &st,
		StatePath: statePath,
		Logger:    logger,
	})))
	app.RegisterService(application.NewService(services.NewServicesService(services.ServicesDeps{
		Sup:    sup,
		Logger: logger,
	})))
	app.RegisterService(application.NewService(services.NewLogsService(services.LogsDeps{
		Sup:    sup,
		Logger: logger,
	})))

	services.RegisterLogStreams(app, sup)
	stopEvents = services.ForwardServiceEvents(app, sup)

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "HyPHP",
		Width:            1200,
		Height:           760,
		MinWidth:         960,
		MinHeight:        600,
		Frameless:        true,
		BackgroundColour: application.NewRGB(15, 24, 38),
		URL:              "/",
	})

	// Fechar a janela oculta para o tray; sair de verdade é pelo menu do tray (ou AppService.Quit).
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if quitting.Load() {
			return
		}
		e.Cancel()
		window.Hide()
	})

	tray = app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("HyPHP")

	menu := app.NewMenu()
	menu.Add("Abrir").OnClick(func(ctx *application.Context) {
		showMain()
	})
	menu.AddSeparator()
	menu.Add("Sair").OnClick(func(ctx *application.Context) {
		quit()
	})
	tray.SetMenu(menu)

	if err := app.Run(); err != nil {
		logger.Error("app encerrou com erro", "err", err)
		os.Exit(1)
	}
}
