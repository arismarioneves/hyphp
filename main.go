package main

import (
	"context"
	"embed"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
	"hyphp/internal/pkgmgr"
	"hyphp/internal/project"
	"hyphp/internal/runtime"
	"hyphp/internal/stack"
	"hyphp/internal/state"
	"hyphp/internal/supervisor"
	"hyphp/internal/webserver"
	"hyphp/internal/webserver/apache"
	"hyphp/internal/webserver/nginx"
	"hyphp/services"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var trayIcon []byte

// opTimeout cobre Reconcile/StartAll/StopAll disparados por tray, boot e shutdown.
const opTimeout = 60 * time.Second

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

	// Job Object primeiro: todo filho nasce dentro dele (spec §7.1).
	sup, err := supervisor.New(logger)
	if err != nil {
		logger.Error("criar supervisor", "err", err)
		os.Exit(1)
	}

	var (
		window  *application.WebviewWindow
		tray    *application.SystemTray
		stk     *stack.Stack
		watcher *project.Watcher
		stopFwd func()
	)

	app := application.New(application.Options{
		Name:        "HyPHP",
		Description: "Ambiente de desenvolvimento PHP",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.hyphp",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if window != nil {
					window.Show()
					window.Restore()
					window.Focus()
				}
			},
		},
		OnShutdown: func() {
			// hyphp: shutdown — ordem inversa do boot
			if watcher != nil {
				_ = watcher.Close()
			}
			if stopFwd != nil {
				stopFwd()
			}
			if stk != nil {
				ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
				defer cancel()
				if err := stk.StopAll(ctx); err != nil {
					logger.Warn("parar stack no shutdown", "err", err)
				}
			}
			if err := sup.Close(); err != nil {
				logger.Warn("fechar supervisor", "err", err)
			}
			if tray != nil {
				tray.Destroy()
			}
		},
	})

	emit := func(name string, data any) { app.Event.Emit(name, data) }

	// hyphp: 06 — runtimes (Scan de bin/ acontece dentro de Installed())
	catalog, err := pkgmgr.LoadEmbedded()
	if err != nil {
		logger.Error("carregar catálogo embutido", "err", err)
		os.Exit(1)
	}
	tmpDir := filepath.Join(paths.Var(), "tmp")
	reconcile := func(reason string) {
		ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
		defer cancel()
		if _, err := stk.Reconcile(ctx); err != nil {
			logger.Error("reconcile", "reason", reason, "err", err)
		}
	}
	rtSvc := services.NewRuntimesService(services.RuntimesDeps{
		BinDir:    paths.Bin(),
		TmpDir:    tmpDir,
		Manager:   pkgmgr.NewManager(paths.Bin(), tmpDir, http.DefaultClient),
		Catalog:   catalog,
		State:     &st,
		StatePath: statePath,
		Logger:    logger,
		Emit:      emit,
		OnChange: func(list []runtime.Installed) {
			// O primeiro Installed() acontece ANTES de stack.New (o Stack precisa
			// da lista de runtimes para escolher o web server). Nesse instante não
			// há nada a reconciliar: o Reconcile inicial roda depois, no bootstrap.
			if stk == nil {
				return
			}
			stk.SetRuntimes(list)
			reconcile("runtime:changed")
		},
	})
	rts := rtSvc.Installed()

	// hyphp: 06 — alocador de portas persistido
	alloc := netcfg.NewAllocator(9000, st.PortAlloc)

	// hyphp: 06 — web servers disponíveis (só os instalados em bin/)
	web := map[state.WebServerName]webserver.WebServer{}
	if list := runtime.ByKind(rts, runtime.Apache); len(list) > 0 {
		web[state.Apache] = apache.New(list[0])
	}
	if list := runtime.ByKind(rts, runtime.Nginx); len(list) > 0 {
		web[state.Nginx] = nginx.New(list[0])
	}

	// hyphp: 06 — mkcert (ausente → sites só em HTTP, warning tls-unavailable)
	mk, err := netcfg.NewMkcert(filepath.Join(paths.Bin(), "mkcert", "mkcert.exe"), filepath.Join(paths.Var(), "certs"))
	if err != nil {
		logger.Warn("mkcert indisponível; sites sem TLS", "err", err)
		mk = netcfg.Mkcert{}
	}

	// hyphp: 06 — stack + projetos
	stk = stack.New(stack.Deps{
		Sup:       sup,
		State:     &st,
		StatePath: statePath,
		Runtimes:  rts,
		Web:       web,
		Alloc:     alloc,
		Mkcert:    mk,
		Logger:    logger,
		Emit:      emit,
	})
	projs, err := project.Discover(st.Roots)
	if err != nil {
		logger.Warn("descobrir projetos", "err", err)
	}
	stk.SetProjects(projs)

	// hyphp: 06 — watcher de roots/projetos. O callback captura a variável
	// projSvc, atribuída logo abaixo; o watcher só dispara após SetRoots, quando
	// projSvc já existe. Se NewWatcher falhar, o service recebe nil e o Rescan
	// manual continua disponível.
	var projSvc *services.ProjectsService
	watcher, err = project.NewWatcher(func() {
		if err := projSvc.Rescan(); err != nil {
			logger.Warn("rescan por watcher", "err", err)
		}
	})
	if err != nil {
		logger.Warn("watcher indisponível; use Rescan manual", "err", err)
		watcher = nil
	}
	projSvc = services.NewProjectsService(stk, watcher, emit)
	if watcher != nil {
		if err := watcher.SetRoots(st.Roots); err != nil {
			logger.Warn("observar roots", "err", err)
		}
	}

	// hyphp: services
	app.RegisterService(application.NewService(services.NewAppService(services.AppDeps{
		Quit: app.Quit, State: &st, StatePath: statePath, Logger: logger,
	})))
	app.RegisterService(application.NewService(projSvc))
	app.RegisterService(application.NewService(services.NewServicesService(services.ServicesDeps{Sup: sup, Logger: logger})))
	app.RegisterService(application.NewService(rtSvc))
	app.RegisterService(application.NewService(services.NewSettingsService(stk, emit)))
	app.RegisterService(application.NewService(services.NewLogsService(services.LogsDeps{Sup: sup, Logger: logger})))
	services.RegisterLogStreams(app, sup)
	stopFwd = services.ForwardServiceEvents(app, sup)

	// janela (plano 01)
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
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		window.Hide()
	})

	// tray (plano 01 + itens "Iniciar tudo"/"Parar tudo" deste plano)
	tray = app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("HyPHP")
	menu := app.NewMenu()
	menu.Add("Abrir").OnClick(func(*application.Context) {
		window.Show()
		window.Focus()
	})
	menu.Add("Iniciar tudo").OnClick(func(*application.Context) { // hyphp: 06
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
			defer cancel()
			if err := stk.StartAll(ctx); err != nil {
				logger.Warn("iniciar tudo (tray)", "err", err)
			}
		}()
	})
	menu.Add("Parar tudo").OnClick(func(*application.Context) { // hyphp: 06
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
			defer cancel()
			if err := stk.StopAll(ctx); err != nil {
				logger.Warn("parar tudo (tray)", "err", err)
			}
		}()
	})
	menu.AddSeparator()
	menu.Add("Sair").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)

	// hyphp: 06 — subir a stack depois que a janela existe (a UI já escuta
	// service:state / stack:warnings / project:changed no primeiro render).
	go func() {
		reconcile("boot")
		ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
		defer cancel()
		if err := stk.StartAll(ctx); err != nil {
			logger.Warn("iniciar stack no boot", "err", err)
		}
	}()

	if err := app.Run(); err != nil {
		logger.Error("app.Run", "err", err)
		os.Exit(1)
	}
}
