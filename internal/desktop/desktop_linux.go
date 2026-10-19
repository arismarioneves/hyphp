package desktop

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
)

// trayWatcher é quem recebe os ícones StatusNotifierItem, como o do Wails. O
// GNOME só o tem com a extensão AppIndicator (padrão no Ubuntu); o Cinnamon e
// o KDE têm sempre.
const trayWatcher = "org.kde.StatusNotifierWatcher"

// callTimeout limita as chamadas ao D-Bus: o portal pode ser iniciado na
// primeira chamada, mas uma sessão sem portal não pode travar a abertura do app.
const callTimeout = 3 * time.Second

// TrayHost diz se há bandeja na sessão. Sem D-Bus de sessão, não há.
func TrayHost() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var has bool
	if err := conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, trayWatcher).Store(&has); err != nil {
		return false
	}
	return has
}

// WatchTrayHost chama fn com o estado da bandeja logo depois de começar a
// vigiar e de novo sempre que ela aparece ou some (a extensão ligada ou
// desligada, o painel reiniciado), até ctx acabar ou a conexão cair. As
// chamadas saem de uma goroutine só, em ordem. A conexão de sessão é a mesma
// do Wails e não é fechada aqui.
func WatchTrayHost(ctx context.Context, fn func(present bool)) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	match := []dbus.MatchOption{
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, trayWatcher),
	}
	if err := conn.AddMatchSignal(match...); err != nil {
		return err
	}
	ch := make(chan *dbus.Signal, 8)
	conn.Signal(ch)
	go func() {
		defer func() {
			conn.RemoveSignal(ch)
			_ = conn.RemoveMatchSignal(match...)
		}()
		// A leitura vem depois de assinar o sinal: uma mudança no meio chega
		// pelo canal, depois dela.
		fn(TrayHost())
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-ch:
				// O godbus fecha o canal quando a conexão cai.
				if !ok {
					return
				}
				// O canal recebe todo sinal da conexão compartilhada.
				if sig.Name != "org.freedesktop.DBus.NameOwnerChanged" || len(sig.Body) != 3 {
					continue
				}
				if name, _ := sig.Body[0].(string); name != trayWatcher {
					continue
				}
				owner, _ := sig.Body[2].(string)
				fn(owner != "")
			}
		}
	}()
	return nil
}

// SystemDark diz se o usuário escolheu o tema escuro, pelo
// org.freedesktop.appearance color-scheme do portal (o mesmo que o GNOME, o
// Cinnamon e o KDE gravam). O WebKitGTK não segue esse valor no
// prefers-color-scheme, por isso ele vem daqui. Sem portal, escuro, o tema
// padrão do HyPHP.
func SystemDark() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	portal := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	var v dbus.Variant
	err = portal.CallWithContext(ctx, "org.freedesktop.portal.Settings.ReadOne", 0, "org.freedesktop.appearance", "color-scheme").Store(&v)
	if err != nil {
		// Portal anterior ao ReadOne: o Read devolve variant dentro de variant.
		err = portal.CallWithContext(ctx, "org.freedesktop.portal.Settings.Read", 0, "org.freedesktop.appearance", "color-scheme").Store(&v)
	}
	if err != nil {
		return true
	}
	return colorSchemeDark(v)
}

// colorSchemeDark lê o color-scheme (0 sem preferência, 1 escuro, 2 claro).
// Sem preferência é o modo claro do GNOME e o padrão dos apps GTK, então vira
// claro. Um tipo ou valor fora disso fica no escuro, o tema padrão do HyPHP.
func colorSchemeDark(v dbus.Variant) bool {
	for {
		inner, ok := v.Value().(dbus.Variant)
		if !ok {
			break
		}
		v = inner
	}
	scheme, ok := v.Value().(uint32)
	return !ok || (scheme != 0 && scheme != 2)
}
