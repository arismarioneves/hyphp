package supervisor

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// donoMorto é o pid do HyPHP que gravou o registro: nenhum processo vivo tem
// esse pid.
const donoMorto = 1 << 30

// grupoSemDono inicia um grupo próprio com dois sleeps (o líder vira sleep
// pelo exec do shell) e devolve o cmd e o retrato do líder já como sleep.
func grupoSemDono(t *testing.T) (*exec.Cmd, procInfo) {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", "sleep 300 & exec sleep 300")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		info := readProcInfo(pid)
		if strings.HasSuffix(info.exe, "/sleep") {
			return cmd, info
		}
		if time.Now().After(deadline) {
			t.Fatalf("o líder %d não virou sleep: %+v", pid, info)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestReapOrphansNoLinuxEncerraOGrupoSemDono(t *testing.T) {
	runDir := t.TempDir()
	cmd, info := grupoSemDono(t)
	pid := cmd.Process.Pid
	r := procRecord{ID: "teste:orfao", PID: pid, PGID: pid, Exe: info.exe, StartSec: info.startSec, OwnerPID: donoMorto}
	if err := writeRecord(runDir, r); err != nil {
		t.Fatal(err)
	}

	if got := ReapOrphans(runDir, nil); len(got) != 1 || got[0] != r.ID {
		t.Fatalf("ReapOrphans = %v; quero [%s]", got, r.ID)
	}
	_ = cmd.Wait() // colhe o líder; o outro sleep é colhido por quem o adotou
	deadline := time.Now().Add(5 * time.Second)
	for !errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH) {
		if time.Now().After(deadline) {
			t.Fatal("o grupo continua vivo depois do ReapOrphans")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(recordPath(runDir, r.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("registro continua lá: %v", err)
	}
}

// Com o dono registrado como pai, o grupo é de um HyPHP vivo: o registro sai
// e nada morre.
func TestReapOrphansNoLinuxPreservaGrupoComODonoVivo(t *testing.T) {
	runDir := t.TempDir()
	cmd, info := grupoSemDono(t)
	pid := cmd.Process.Pid
	r := procRecord{ID: "teste:vivo", PID: pid, PGID: pid, Exe: info.exe, StartSec: info.startSec, OwnerPID: os.Getpid()}
	if err := writeRecord(runDir, r); err != nil {
		t.Fatal(err)
	}

	if got := ReapOrphans(runDir, nil); len(got) != 0 {
		t.Fatalf("ReapOrphans = %v; quero nada", got)
	}
	if exited(pid) {
		t.Fatal("processo com o dono vivo foi morto")
	}
}
