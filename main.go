package main

import (
	"context"
	"embed"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"hyphp/internal/autostart"
	"hyphp/internal/i18n"
	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
	"hyphp/internal/pkgmgr"
	"hyphp/internal/project"
	"hyphp/internal/runtime"
	"hyphp/internal/stack"
	"hyphp/internal/state"
	"hyphp/internal/supervisor"
	"hyphp/internal/update"
	"hyphp/internal/version"
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
	// Modo do atualizador (spec do update §3): roda de uma cópia em
	// var/update/, antes de log, paths e Wails. Tratado aqui em cima porque
	// qualquer uma dessas coisas calcularia a raiz a partir da pasta da cópia.
	if len(os.Args) > 1 && os.Args[1] == update.ApplyFlag {
		os.Exit(update.RunApply(os.Args[2:]))
	}

	// A raiz fica fixa a partir daqui: uma variável de ambiente ou o cwd
	// mudando no meio da execução não pode espalhar o estado em duas pastas.
	paths.Freeze()

	// O log vai para log/hyphp.log e, havendo console, também para stderr.
	// Sob o Wails em modo GUI não há console, então sem o arquivo qualquer
	// falha de bootstrap fica invisível — inclusive para a tela Logs.
	logOut := io.Writer(os.Stderr)
	if err := paths.EnsureLayout(); err != nil {
		slog.Error("criar layout de runtime", "root", paths.Root(), "err", err)
		os.Exit(1)
	}
	if f, err := os.OpenFile(filepath.Join(paths.Log(), "hyphp.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		defer f.Close()
		logOut = logOutput(os.Stderr, f)
	}
	logger := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Info("hyphp iniciando", "root", paths.Root())
	statePath := filepath.Join(paths.Var(), "state.json")
	st, recovered, err := state.LoadOrRecover(statePath)
	if err != nil {
		logger.Error("carregar state.json", "path", statePath, "err", err)
		os.Exit(1)
	}
	if recovered != "" {
		logger.Warn("state.json corrompido; guardado à parte e recriado com o padrão", "path", statePath, "backup", recovered)
	}
	// Antes de qualquer texto do Go: avisos, tray e diálogos saem no idioma
	// escolhido (ou no do Windows).
	i18n.SetCurrent(i18n.Resolve(st.Language))

	// Reconciliar o autostart com o registro. Dois casos reais que de outro
	// modo quebram calados:
	//
	// 1. O usuário desativa o HyPHP no Gerenciador de Tarefas > Inicializar.
	//    A entrada some do registro, mas o state.json continuaria dizendo que
	//    está ligado — e o toggle da tela mentiria. Aqui o sistema vence.
	// 2. O app é reinstalado em outro diretório. A entrada aponta para o exe
	//    antigo e o autostart simplesmente não abre nada; regravar com o
	//    caminho atual conserta.
	if on, aerr := autostart.Enabled(); aerr != nil {
		logger.Warn("ler autostart do registro", "err", aerr)
	} else if on != st.Autostart {
		st.Autostart = on
		if serr := state.Save(statePath, st); serr != nil {
			logger.Warn("persistir autostart reconciliado", "err", serr)
		}
	} else if on {
		if aerr := autostart.Apply(true); aerr != nil {
			logger.Warn("atualizar caminho do autostart", "err", aerr)
		}
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
		stopUpd context.CancelFunc
		updItem *application.MenuItem
		// relabelTray reescreve o menu no idioma atual; no-op até o tray existir.
		relabelTray = func() {}
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
			if stopUpd != nil {
				stopUpd()
			}
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
			if stk != nil {
				// O resolvedor DNS precisa soltar a porta 53; se ficasse preso,
				// a próxima execução falharia no bind.
				if err := stk.Close(); err != nil {
					logger.Warn("fechar stack", "err", err)
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
		App:    app,
		BinDir: paths.Bin(),
		TmpDir: tmpDir,
		// nil: o pkgmgr monta um client com timeout de resposta; o
		// http.DefaultClient esperava para sempre um CDN que parou de responder.
		Manager: pkgmgr.NewManager(paths.Bin(), tmpDir, nil),
		Catalog: catalog,
		// Só chamados depois de stack.New: antes dele o serviço apenas varre bin/.
		UpdateState: func(fn func(*state.State)) error { return stk.UpdateState(fn) },
		State:       func() state.State { return stk.State() },
		Logger:      logger,
		Emit:        emit,
		OnChange: func(list []runtime.Installed) {
			// O primeiro Installed() acontece ANTES de stack.New (o Stack precisa
			// da lista de runtimes para escolher o web server). Nesse instante não
			// há nada a reconciliar: o Reconcile inicial roda depois, no bootstrap.
			if stk == nil {
				return
			}
			stk.SetRuntimes(list)
			// Web server e mkcert são derivados do que existe em bin/. Sem
			// recalcular aqui, instalar o Apache (ou o mkcert) com o app aberto
			// não tinha efeito nenhum até reiniciar: o mapa continuava o do boot
			// e o aviso "não encontrado" persistia com o runtime já instalado.
			stk.SetWebServers(webServers(list))
			stk.SetMkcert(newMkcert(logger))
			reconcile("runtime:changed")
		},
		StopUsing: func(dir string) error {
			if stk == nil {
				return nil
			}
			return stk.StopUsingDir(dir)
		},
	})
	rts := rtSvc.Installed()

	// hyphp: 06 — alocador de portas persistido
	alloc := netcfg.NewAllocator(9000, st.PortAlloc)

	// hyphp: 06 — web servers e mkcert disponíveis (só os instalados em bin/)
	web := webServers(rts)
	mk := newMkcert(logger)

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
	// Discover devolve os projetos válidos mesmo quando algum falha; o erro
	// lista só os ignorados.
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
	projSvc = services.NewProjectsService(app, stk, watcher, emit, logger)
	if watcher != nil {
		if err := watcher.SetRoots(st.Roots); err != nil {
			logger.Warn("observar roots", "err", err)
		}
	}

	// hyphp: 11 — auto-update. Em build de dev fica "inativo" (BuildEnabled).
	updURL, err := update.DefaultURL()
	if err != nil {
		logger.Warn("URL de update inválida; usando a oficial", "err", err)
		updURL = update.ManifestURL
	}
	upd := update.New(update.Config{
		Current:   version.Current,
		URL:       updURL,
		Dir:       filepath.Join(paths.Var(), "update"),
		Key:       update.PublicKey,
		Client:    &http.Client{Timeout: 30 * time.Minute},
		Enabled:   update.BuildEnabled,
		AutoCheck: func() bool { return !stk.State().AutoUpdateOff },
		OnStatus: func(s update.Status) {
			emit("update:status", s)
			syncUpdateItem(updItem, s)
		},
		Logger: logger,
	})

	// hyphp: services
	appSvc := services.NewAppService(services.AppDeps{
		Quit: app.Quit, State: stk.State, Logger: logger,
		// Mesma regra do Reconcile (stack/desired.go): state.DefaultPHP manda e,
		// vazio, vale a maior série instalada. O PATH do usuário precisa apontar
		// para o PHP que a stack de fato serve.
		DefaultPHP: func() (runtime.Installed, bool) {
			phps := runtime.ByKind(rtSvc.Installed(), runtime.PHP)
			major := stk.State().DefaultPHP
			if major == "" {
				major = stack.HighestPHPMajor(phps)
			}
			return runtime.PHPByMajor(phps, major)
		},
	})
	svcSvc := services.NewServicesService(services.ServicesDeps{Sup: sup, Stack: stk, Logger: logger})
	setSvc := services.NewSettingsService(stk, emit, func() { relabelTray() })
	logsSvc := services.NewLogsService(services.LogsDeps{Sup: sup, Logger: logger})
	dbSvc := services.NewDatabaseService(services.DatabaseDeps{
		Sup:      sup,
		Stack:    stk,
		Runtimes: rtSvc.Installed, // method value: Scan cacheado do RuntimesService
		Logger:   logger,
	})
	app.RegisterService(application.NewService(appSvc))
	app.RegisterService(application.NewService(projSvc))
	app.RegisterService(application.NewService(svcSvc))
	app.RegisterService(application.NewService(rtSvc))
	app.RegisterService(application.NewService(setSvc))
	app.RegisterService(application.NewService(logsSvc))
	app.RegisterService(application.NewService(dbSvc))
	app.RegisterService(application.NewService(services.NewUpdateService(upd, app.Quit)))
	// A CLI (cmd/hyphp) fala com o app pelo pipe do usuário e roda os mesmos
	// serviços que a UI; o hyphp.exe dela mora em cli\ ao lado do app.
	exe, _ := os.Executable()
	app.RegisterService(application.NewService(services.NewCLIService(services.CLIDeps{
		App: appSvc, Services: svcSvc, Projects: projSvc, Runtimes: rtSvc, Settings: setSvc, Database: dbSvc, Sup: sup,
		ShowWindow: func() {
			if window != nil {
				window.Show()
				window.Restore()
				window.Focus()
			}
		},
		Emit:   emit,
		Logger: logger,
		Exe:    filepath.Join(filepath.Dir(exe), "cli", "hyphp.exe"),
	})))
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
		BackgroundColour: windowBackground(st.Theme, systemDarkMode()),
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
	showWindow := func() {
		window.Show()
		window.Restore()
		window.Focus()
	}
	// Clique esquerdo abre o app, como em todo ícone de bandeja do Windows; o
	// padrão do Wails era não fazer nada, e só o menu do clique direito abria.
	tray.OnClick(showWindow)
	menu := app.NewMenu()
	openItem := menu.Add(i18n.T("tray.open")).OnClick(func(*application.Context) { showWindow() })
	startItem := menu.Add(i18n.T("tray.startAll")).OnClick(func(*application.Context) { // hyphp: 06
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
			defer cancel()
			if err := stk.StartAll(ctx); err != nil {
				logger.Warn("iniciar tudo (tray)", "err", err)
			}
		}()
	})
	stopItem := menu.Add(i18n.T("tray.stopAll")).OnClick(func(*application.Context) { // hyphp: 06
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
			defer cancel()
			if err := stk.StopAll(ctx); err != nil {
				logger.Warn("parar tudo (tray)", "err", err)
			}
		}()
	})
	// Só a ação que faz sentido agora: com algo rodando, Parar; com tudo
	// parado, Iniciar. Decide na hora de abrir o menu, que é quando o estado
	// importa, sem assinar cada transição dos serviços.
	syncStartStop := func() {
		running := anyRunning(sup.List())
		startItem.SetHidden(running)
		stopItem.SetHidden(!running)
	}
	syncStartStop()
	tray.OnRightClick(func() {
		syncStartStop()
		tray.OpenMenu()
	})
	// Oculto até haver versão pronta; syncUpdateItem mostra e rotula.
	updItem = menu.Add(i18n.T("tray.update")).SetHidden(true).OnClick(func(*application.Context) {
		go func() {
			if err := upd.Apply(app.Quit); err != nil {
				logger.Warn("aplicar atualização (tray)", "err", err)
			}
		}()
	})
	menu.AddSeparator()
	quitItem := menu.Add(i18n.T("tray.quit")).OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)
	// Trocar o idioma em Configurações reescreve o menu; o rótulo do item de
	// update depende da versão pronta, e syncUpdateItem cuida dele.
	relabelTray = func() {
		application.InvokeAsync(func() {
			openItem.SetLabel(i18n.T("tray.open"))
			startItem.SetLabel(i18n.T("tray.startAll"))
			stopItem.SetLabel(i18n.T("tray.stopAll"))
			quitItem.SetLabel(i18n.T("tray.quit"))
			syncUpdateItem(updItem, upd.Status())
		})
	}

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

	// O updater só começa depois que o Wails sobe: o primeiro OnStatus pode
	// sair já no Start (resultado do update anterior), e syncUpdateItem usa
	// InvokeAsync, que antes de app.Run() desreferencia um impl nil e derruba
	// o processo — exatamente no boot que vem logo depois de um update.
	updCtx, cancelUpd := context.WithCancel(context.Background())
	stopUpd = cancelUpd
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		upd.Start(updCtx)
	})

	if err := app.Run(); err != nil {
		logger.Error("app.Run", "err", err)
		os.Exit(1)
	}
}

// syncUpdateItem mostra o item do tray só com versão pronta. Roda na thread
// de UI porque OnStatus dispara das goroutines de verificação e download.
func syncUpdateItem(item *application.MenuItem, s update.Status) {
	if item == nil {
		return
	}
	application.InvokeAsync(func() {
		ready := s.State == update.StateReady
		item.SetHidden(!ready)
		if ready {
			item.SetLabel(i18n.T("tray.updateTo", s.Available))
		}
	})
}

// windowBackground é a cor da janela antes de o WebView pintar: sem casar com
// o tema, abrir o app no tema claro piscaria o fundo escuro.
func windowBackground(theme string, systemDark bool) application.RGBA {
	if theme == state.ThemeLight || (theme == state.ThemeSystem && !systemDark) {
		return application.NewRGB(245, 247, 251) // --bg-app do tema claro
	}
	return application.NewRGB(15, 24, 38) // --bg-app do tema escuro
}

// anyRunning diz se algum serviço está no ar ou a caminho. Stopping conta:
// oferecer "Iniciar tudo" enquanto a parada ainda termina faria o clique
// esbarrar em "serviço já está stopping".
func anyRunning(list []supervisor.Status) bool {
	for _, s := range list {
		if s.State != supervisor.Stopped && s.State != supervisor.Failed {
			return true
		}
	}
	return false
}

// webServers monta o mapa de web servers a partir do que está instalado em
// bin/. É função (e não código inline no bootstrap) porque precisa rodar de
// novo a cada mudança em bin/: instalar o Apache com o app aberto tem de
// passar a valer sem reiniciar.
func webServers(rts []runtime.Installed) map[state.WebServerName]webserver.WebServer {
	web := map[state.WebServerName]webserver.WebServer{}
	if inst, ok := runtime.Newest(rts, runtime.Apache); ok {
		web[state.Apache] = apache.New(inst)
	}
	if inst, ok := runtime.Newest(rts, runtime.Nginx); ok {
		web[state.Nginx] = nginx.New(inst)
	}
	return web
}

// newMkcert resolve o mkcert em bin/. Ausente não é erro: os sites ficam só em
// HTTP e o Reconcile emite tls-unavailable.
func newMkcert(logger *slog.Logger) netcfg.Mkcert {
	mk, err := netcfg.NewMkcert(filepath.Join(paths.Bin(), "mkcert", "mkcert.exe"), filepath.Join(paths.Var(), "certs"))
	if err != nil {
		logger.Warn("mkcert indisponível; sites sem TLS", "err", err)
		return netcfg.Mkcert{}
	}
	return mk
}

// logOutput devolve o destino do log: o arquivo sempre, e stderr só quando
// ele existe. No build de produção (-H windowsgui) o stderr é um handle
// inválido, e io.MultiWriter para no primeiro writer que falha — com stderr
// na frente, o arquivo nunca recebia nada e log/hyphp.log ficava com 0 bytes.
func logOutput(stderr *os.File, file io.Writer) io.Writer {
	if _, err := stderr.Stat(); err != nil {
		return file
	}
	return io.MultiWriter(file, stderr)
}
