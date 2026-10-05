package supervisor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestAjudanteOrfao não é um teste: é o processo intermediário de
// iniciarOrfao, rodado pelo próprio binário de teste. Ele inicia um
// `sh -c 'sleep 60 & wait'` no próprio grupo (Setpgid, como o
// startProcess), imprime o pid e sai sem esperar. O sh fica líder do grupo
// e, com o pai morto, é reparentado ao launchd: exatamente o órfão que um
// crash do app deixa. O macOS não tem setsid(1) nem subreaper, e um `&` no
// sh não troca de grupo sem job control (que exige tty), por isso o Go.
func TestAjudanteOrfao(t *testing.T) {
	if os.Getenv("HYPHP_TESTE_ORFAO") != "1" {
		return
	}
	cmd := exec.Command("/bin/sh", "-c", "sleep 60 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Printf("pid=%d\n", cmd.Process.Pid)
	os.Exit(0)
}

// iniciarOrfao devolve o pid de um sh órfão (ppid 1) que lidera o próprio
// grupo, com um sleep filho no mesmo grupo.
func iniciarOrfao(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestAjudanteOrfao$")
	cmd.Env = append(os.Environ(), "HYPHP_TESTE_ORFAO=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ajudante: %v (%s)", err, out)
	}
	pid := 0
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "pid="); ok {
			pid, _ = strconv.Atoi(v)
		}
	}
	if pid <= 1 {
		t.Fatalf("ajudante não informou o pid: %q", out)
	}
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
	deadline := time.Now().Add(5 * time.Second)
	for readProcInfo(pid).ppid != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("pid %d não foi reparentado ao launchd: %+v", pid, readProcInfo(pid))
		}
		time.Sleep(20 * time.Millisecond)
	}
	return pid
}

// registrarComo grava o registro que o supervisor gravaria para o pid.
func registrarComo(t *testing.T, runDir, id string, pid int, exe string) procRecord {
	t.Helper()
	info := readProcInfo(pid)
	if !info.alive {
		t.Fatalf("pid %d não está vivo", pid)
	}
	r := procRecord{ID: id, PID: pid, PGID: pid, Exe: exe, StartSec: info.startSec, StartUsec: info.startUsec, OwnerPID: 1}
	if err := writeRecord(runDir, r); err != nil {
		t.Fatalf("writeRecord: %v", err)
	}
	return r
}

func registroExiste(t *testing.T, runDir, id string) bool {
	t.Helper()
	_, err := os.Stat(recordPath(runDir, id))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat do registro: %v", err)
	}
	return err == nil
}

// grupoVivo diz se algum membro do grupo ainda roda (zumbi não conta).
func grupoVivo(t *testing.T, pgid int) bool {
	t.Helper()
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pgid)
	if err != nil {
		t.Fatalf("kern.proc.pgrp: %v", err)
	}
	for _, p := range procs {
		if p.Proc.P_stat != sZomb {
			return true
		}
	}
	return false
}

func TestReapOrphansEncerraOGrupoOrfaoInteiro(t *testing.T) {
	runDir := t.TempDir()
	pid := iniciarOrfao(t)
	registrarComo(t, runDir, "web:apache", pid, "/bin/sh")
	if !grupoVivo(t, pid) {
		t.Fatal("o grupo do órfão já nasceu morto")
	}

	got := ReapOrphans(runDir, nil)

	if !slices.Equal(got, []string{"web:apache"}) {
		t.Fatalf("ReapOrphans = %v, want [web:apache]", got)
	}
	deadline := time.Now().Add(2 * time.Second)
	for grupoVivo(t, pid) {
		if time.Now().After(deadline) {
			t.Fatalf("grupo %d ainda tem membros vivos", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if registroExiste(t, runDir, "web:apache") {
		t.Fatal("registro do órfão encerrado não foi apagado")
	}
}

func TestReapOrphansComHorarioDiferenteNaoMataEApagaORegistro(t *testing.T) {
	runDir := t.TempDir()
	pid := iniciarOrfao(t)
	r := registrarComo(t, runDir, "web:apache", pid, "/bin/sh")
	// Mesmo pid, outro início: é o caso do pid reciclado por outro processo.
	r.StartSec--
	if err := writeRecord(runDir, r); err != nil {
		t.Fatal(err)
	}

	if got := ReapOrphans(runDir, nil); len(got) != 0 {
		t.Fatalf("ReapOrphans = %v, want nada", got)
	}
	if !processAlive(pid) || !grupoVivo(t, pid) {
		t.Fatal("processo com horário de início diferente foi morto")
	}
	if registroExiste(t, runDir, "web:apache") {
		t.Fatal("registro que não bate continuou no disco")
	}
}

func TestReapOrphansComExeDiferenteNaoMata(t *testing.T) {
	runDir := t.TempDir()
	pid := iniciarOrfao(t)
	// pid, início, grupo e ppid 1 batem, como num `brew services` com o
	// mesmo pid; o executável não.
	registrarComo(t, runDir, "web:apache", pid, "/opt/homebrew/opt/httpd/bin/httpd")

	if got := ReapOrphans(runDir, nil); len(got) != 0 {
		t.Fatalf("ReapOrphans = %v, want nada", got)
	}
	if !processAlive(pid) {
		t.Fatal("processo com outro executável foi morto")
	}
}

func TestReapOrphansPreservaProcessoComPaiVivo(t *testing.T) {
	runDir := t.TempDir()
	sup := newTestSupervisor(t)
	sup.SetRunDir(runDir)
	spec := longRunningSpec("srv:pai-vivo")
	if err := sup.Add(spec); err != nil {
		t.Fatal(err)
	}
	if err := sup.Start(spec.ID); err != nil {
		t.Fatal(err)
	}
	st, _ := sup.Status(spec.ID)
	if !registroExiste(t, runDir, spec.ID) {
		t.Fatal("Start não gravou o registro")
	}

	if got := ReapOrphans(runDir, nil); len(got) != 0 {
		t.Fatalf("ReapOrphans = %v, want nada", got)
	}
	if !processAlive(st.PID) {
		t.Fatal("processo com o pai (este teste) vivo foi morto")
	}
}

func TestRegistroNasceNoStartESomeNoStop(t *testing.T) {
	runDir := t.TempDir()
	sup := newTestSupervisor(t)
	sup.SetRunDir(runDir)
	spec := longRunningSpec("srv:registro")
	if err := sup.Add(spec); err != nil {
		t.Fatal(err)
	}
	if err := sup.Start(spec.ID); err != nil {
		t.Fatal(err)
	}
	st, _ := sup.Status(spec.ID)

	data, err := os.ReadFile(recordPath(runDir, spec.ID))
	if err != nil {
		t.Fatalf("registro: %v", err)
	}
	var r procRecord
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	info := readProcInfo(st.PID)
	if r.ID != spec.ID || r.PID != st.PID || r.PGID != st.PID || r.Exe != "/bin/sh" || r.OwnerPID != os.Getpid() ||
		r.StartSec != info.startSec || r.StartUsec != info.startUsec {
		t.Fatalf("registro = %+v; processo pid=%d início=%d.%06d", r, st.PID, info.startSec, info.startUsec)
	}
	// O exec path do kern.procargs2 é o que o registro guardou.
	if info.exe != r.Exe {
		t.Fatalf("kern.procargs2 = %q, registro = %q", info.exe, r.Exe)
	}

	if err := sup.Stop(spec.ID); err != nil {
		t.Fatal(err)
	}
	if registroExiste(t, runDir, spec.ID) {
		t.Fatal("Stop não apagou o registro")
	}
}

func TestReapOrphansSemPastaProcsNaoFazNada(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "não-existe")
	if got := ReapOrphans(runDir, nil); len(got) != 0 {
		t.Fatalf("ReapOrphans = %v, want nada", got)
	}
}

func TestSameExecutableAceitaOCaminhoPorSymlink(t *testing.T) {
	// Como o opt/ do Homebrew, que aponta para a versão no Cellar.
	dir := t.TempDir()
	alvo := filepath.Join(dir, "Cellar", "httpd")
	if err := os.MkdirAll(filepath.Dir(alvo), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(alvo, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "opt-httpd")
	if err := os.Symlink(alvo, link); err != nil {
		t.Fatal(err)
	}
	if !sameExecutable(link, alvo) || !sameExecutable(alvo, link) {
		t.Fatal("o mesmo binário por symlink não bateu")
	}
	if sameExecutable(link, "/bin/sh") {
		t.Fatal("binários diferentes bateram")
	}
}
