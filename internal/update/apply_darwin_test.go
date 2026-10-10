package update

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// appFalso cria em dir um HyPHP.app mínimo, com a versão no Info.plist.
func appFalso(t *testing.T, dir, versao string) string {
	t.Helper()
	app := filepath.Join(dir, "HyPHP.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>CFBundleShortVersionString</key><string>` + versao + `</string></dict></plist>
`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "MacOS", "hyphp"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return app
}

// dmgFalso empacota um HyPHP.app da versão dada num dmg de verdade, com o
// mesmo hdiutil que o atualizador usa para montar.
func dmgFalso(t *testing.T, versao string) (path string, size int64, sum string) {
	t.Helper()
	src := t.TempDir()
	appFalso(t, src, versao)
	path = filepath.Join(t.TempDir(), "hyphp.dmg")
	if out, err := exec.Command("hdiutil", "create", "-quiet", "-srcfolder", src, "-volname", "HyPHP", "-format", "UDZO", "-o", path).CombinedOutput(); err != nil {
		t.Fatalf("hdiutil create: %v: %s", err, out)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := sha256.Sum256(raw)
	return path, int64(len(raw)), hex.EncodeToString(s[:])
}

// semSobras exige que a pasta do app tenha só o HyPHP.app, sem o .novo nem o
// .antigo da troca.
func semSobras(t *testing.T, pasta string) {
	t.Helper()
	entries, err := os.ReadDir(pasta)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "HyPHP.app" {
			t.Errorf("sobrou %s na pasta do app", e.Name())
		}
	}
}

// desmontado exige que o dmg não continue montado: um volume esquecido a
// cada update fica pendurado no Finder até reiniciar o Mac.
func desmontado(t *testing.T, dmg string) {
	t.Helper()
	out, err := exec.Command("hdiutil", "info").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), dmg) {
		t.Errorf("o dmg %s continua montado", dmg)
	}
}

func versaoDe(t *testing.T, app string) string {
	t.Helper()
	v, err := bundleVersion(app)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// PID 0 não é processo nenhum, então waitExit não espera: o teste começa do
// ponto em que o app já saiu.
func TestAplicarTrocaOBundle(t *testing.T) {
	dmg, size, sum := dmgFalso(t, "1.1.0")
	pasta := t.TempDir()
	app := appFalso(t, pasta, "1.0.0")
	req := ApplyRequest{Installer: dmg, SHA256: sum, Size: size, Dir: filepath.Join(app, "Contents", "MacOS"), Exe: "hyphp", To: "1.1.0"}
	if err := apply(req, func(string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	if v := versaoDe(t, app); v != "1.1.0" {
		t.Errorf("app instalado na %s depois do update, quer 1.1.0", v)
	}
	semSobras(t, pasta)
	desmontado(t, dmg)
}

// O dmg baixado traz outra versão que a anunciada no manifesto: aplicar seria
// o começo de um loop de update. O app instalado fica intacto.
func TestAplicarRecusaVersaoDiferente(t *testing.T) {
	dmg, size, sum := dmgFalso(t, "1.2.0")
	pasta := t.TempDir()
	app := appFalso(t, pasta, "1.0.0")
	req := ApplyRequest{Installer: dmg, SHA256: sum, Size: size, Dir: filepath.Join(app, "Contents", "MacOS"), Exe: "hyphp", To: "1.1.0"}
	err := apply(req, func(string, ...any) {})
	if err == nil || !strings.Contains(err.Error(), "1.2.0") {
		t.Fatalf("erro = %v, quer recusa citando a versão do dmg", err)
	}
	if v := versaoDe(t, app); v != "1.0.0" {
		t.Errorf("app instalado mexido: %s", v)
	}
	semSobras(t, pasta)
	desmontado(t, dmg)
}

// O segundo rename falhou com o app atual já afastado: ele volta para o
// lugar, senão a pasta ficaria sem HyPHP.
func TestTrocaComSegundoRenameFalhoDevolveOAntigo(t *testing.T) {
	pasta := t.TempDir()
	app := appFalso(t, pasta, "1.0.0")
	novo := appFalso(t, t.TempDir(), "1.1.0")
	n := 0
	rename := func(oldpath, newpath string) error {
		n++
		if n == 2 {
			return errors.New("rename recusado")
		}
		return os.Rename(oldpath, newpath)
	}
	if err := replaceBundle(novo, app, rename); err == nil {
		t.Fatal("a troca com o segundo rename recusado deu certo")
	}
	if v := versaoDe(t, app); v != "1.0.0" {
		t.Errorf("app no lugar é a %s, quer o antigo (1.0.0)", v)
	}
	semSobras(t, pasta)
}

func TestBundleDoExecutavel(t *testing.T) {
	if b, err := bundleOf("/Applications/HyPHP.app/Contents/MacOS"); err != nil || b != "/Applications/HyPHP.app" {
		t.Errorf("bundleOf = %q, %v", b, err)
	}
	// Fora de um .app (go run, binário de teste) não há bundle para trocar.
	for _, dir := range []string{"/usr/local/bin", "/Applications/HyPHP/Contents/MacOS", "/Applications/HyPHP.app/Contents/Helpers"} {
		if b, err := bundleOf(dir); err == nil {
			t.Errorf("bundleOf(%s) = %q, quer erro", dir, b)
		}
	}
}

// Sem gravação na pasta do app (conta sem administrador em /Applications), o
// app não pode fechar para nada: Launch recusa antes de copiar ou iniciar o
// atualizador, e o erro ensina o comando do Terminal.
func TestLaunchRecusaPastaSemGravacao(t *testing.T) {
	pasta := filepath.Join(t.TempDir(), "Applications")
	app := appFalso(t, pasta, "1.0.0")
	if err := os.Chmod(pasta, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(pasta, 0o755) })
	updater := filepath.Join(t.TempDir(), updaterExe)
	err := Launch(ApplyRequest{Dir: filepath.Join(app, "Contents", "MacOS"), Exe: "hyphp"}, updater)
	if err == nil || !strings.Contains(err.Error(), "install.sh") {
		t.Fatalf("erro = %v, quer a recusa com o comando do install.sh", err)
	}
	if _, err := os.Stat(updater); !os.IsNotExist(err) {
		t.Error("o atualizador foi copiado mesmo com a recusa")
	}
}

// O app sai, e o atualizador segue; se ele não sair no prazo, nada é trocado.
// O Wait em paralelo faz o papel do launchd, que recolhe o app de verdade.
func TestEsperarSaidaDoApp(t *testing.T) {
	curto := exec.Command("sleep", "0.3")
	if err := curto.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = curto.Wait() }()
	if err := waitExit(curto.Process.Pid, 10*time.Second); err != nil {
		t.Errorf("processo que saiu: %v", err)
	}
	longo := exec.Command("sleep", "30")
	if err := longo.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = longo.Process.Kill()
		_ = longo.Wait()
	})
	if err := waitExit(longo.Process.Pid, 300*time.Millisecond); !errors.Is(err, errAppAlive) {
		t.Errorf("processo vivo além do prazo: erro = %v, quer errAppAlive", err)
	}
}
