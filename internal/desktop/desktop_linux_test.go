package desktop

import (
	"context"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// outraConexao é um segundo cliente no bus de sessão do teste (o CI roda os
// testes sob dbus-run-session), no papel do painel ou do portal.
func outraConexao(t *testing.T) *dbus.Conn {
	t.Helper()
	if _, err := dbus.SessionBus(); err != nil {
		t.Skipf("sem D-Bus de sessão: %v", err)
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// pegarNome faz conn dono de name, ou pula o teste quando outro processo da
// sessão já é (uma sessão gráfica de verdade em vez do dbus-run-session).
func pegarNome(t *testing.T, conn *dbus.Conn, name string) {
	t.Helper()
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil {
		t.Fatal(err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		t.Skipf("%s já tem dono nesta sessão; o teste precisa de um bus próprio (dbus-run-session)", name)
	}
}

func esperar(t *testing.T, ch <-chan bool) bool {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("o aviso de mudança da bandeja não chegou")
		return false
	}
}

func TestBandejaApareceESomeComOWatcher(t *testing.T) {
	painel := outraConexao(t)
	if TrayHost() {
		t.Skip("a sessão já tem bandeja; o teste precisa de um bus próprio (dbus-run-session)")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mudou := make(chan bool, 4)
	if err := WatchTrayHost(ctx, func(p bool) { mudou <- p }); err != nil {
		t.Fatal(err)
	}
	if esperar(t, mudou) {
		t.Fatal("o primeiro aviso devia ser o estado de agora, sem bandeja")
	}

	pegarNome(t, painel, trayWatcher)
	if !esperar(t, mudou) || !TrayHost() {
		t.Fatal("o watcher apareceu e a bandeja não")
	}
	if _, err := painel.ReleaseName(trayWatcher); err != nil {
		t.Fatal(err)
	}
	if esperar(t, mudou) || TrayHost() {
		t.Fatal("o watcher saiu e a bandeja continuou")
	}
}

// portalFalso responde o color-scheme como o xdg-desktop-portal.
type portalFalso struct{ scheme uint32 }

func (p *portalFalso) ReadOne(namespace, key string) (dbus.Variant, *dbus.Error) {
	if namespace != "org.freedesktop.appearance" || key != "color-scheme" {
		return dbus.Variant{}, dbus.NewError("org.freedesktop.portal.Error.NotFound", nil)
	}
	return dbus.MakeVariant(p.scheme), nil
}

func TestTemaVemDoPortal(t *testing.T) {
	portal := outraConexao(t)
	p := &portalFalso{scheme: 2}
	if err := portal.Export(p, "/org/freedesktop/portal/desktop", "org.freedesktop.portal.Settings"); err != nil {
		t.Fatal(err)
	}
	pegarNome(t, portal, "org.freedesktop.portal.Desktop")
	if SystemDark() {
		t.Fatal("color-scheme 2 (claro) saiu escuro")
	}
	p.scheme = 1
	if !SystemDark() {
		t.Fatal("color-scheme 1 (escuro) saiu claro")
	}
}

func TestColorSchemeSoOEscuroEValorDesconhecidoSaoEscuros(t *testing.T) {
	casos := []struct {
		nome string
		v    dbus.Variant
		dark bool
	}{
		{"claro", dbus.MakeVariant(uint32(2)), false},
		{"escuro", dbus.MakeVariant(uint32(1)), true},
		{"sem preferência, o claro do GNOME", dbus.MakeVariant(uint32(0)), false},
		{"variant dentro de variant, do Read antigo", dbus.MakeVariant(dbus.MakeVariant(uint32(2))), false},
		{"valor fora de 0, 1 e 2", dbus.MakeVariant(uint32(7)), true},
		{"tipo inesperado", dbus.MakeVariant("claro"), true},
	}
	for _, c := range casos {
		if got := colorSchemeDark(c.v); got != c.dark {
			t.Errorf("%s: colorSchemeDark = %v; quero %v", c.nome, got, c.dark)
		}
	}
}
