package update

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"hyphp/internal/i18n"
)

// O app que sai fica zumbi até quem o abriu recolhê-lo; o atualizador não
// pode esperar por isso.
func TestWaitExitAceitaZumbiComoEncerrado(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// O Wait só no fim: até lá o processo fica zumbi, filho deste teste.
	t.Cleanup(func() { _ = cmd.Wait() })
	inicio := time.Now()
	if err := waitExit(cmd.Process.Pid, 10*time.Second); err != nil {
		t.Fatalf("waitExit = %v", err)
	}
	if d := time.Since(inicio); d > 5*time.Second {
		t.Fatalf("waitExit levou %s para um zumbi", d)
	}
}

func TestWaitExitDesisteDoProcessoVivo(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	if err := waitExit(cmd.Process.Pid, 300*time.Millisecond); !errors.Is(err, errAppAlive) {
		t.Fatalf("waitExit = %v, quer errAppAlive", err)
	}
}

// Fora do pacote o apt-get não troca o app em curso: a recusa vem antes de o
// app fechar, com o link da versão nova.
func TestLaunchRecusaAppForaDoPacote(t *testing.T) {
	dir := t.TempDir()
	err := Launch(ApplyRequest{Dir: dir, Exe: "hyphp"}, filepath.Join(dir, updaterExe))
	if err == nil || err.Error() != i18n.T("update.semPacote", releasesURL) {
		t.Fatalf("erro = %v, quer a recusa do catálogo", err)
	}
}
