package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// No macOS não existe "mate meus filhos se eu morrer": se o app cair, cada
// serviço segue vivo no próprio grupo, reparentado ao launchd. O supervisor
// registra cada processo iniciado e a próxima abertura encerra os que sobraram
// — e só eles (ver shouldReap).

// recordProcess grava o registro do processo recém-iniciado. O horário de
// início vem do kernel (o mesmo campo que o ReapOrphans compara); o
// executável é o cmd.Path, o caminho que foi passado ao execve e que o
// kern.procargs2 devolve. runDir vazio desliga o registro (testes).
func recordProcess(runDir string, spec Spec, cmd *exec.Cmd, h procHandle) error {
	if runDir == "" || cmd == nil || cmd.Process == nil || h.pgid <= 0 {
		return nil
	}
	pid := cmd.Process.Pid
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return fmt.Errorf("ler horário de início do pid %d: %w", pid, err)
	}
	r := procRecord{
		ID:        spec.ID,
		PID:       pid,
		PGID:      h.pgid,
		Exe:       cmd.Path,
		StartSec:  kp.Proc.P_starttime.Sec,
		StartUsec: int64(kp.Proc.P_starttime.Usec),
		OwnerPID:  os.Getpid(),
	}
	return writeRecord(runDir, r)
}

// writeRecord grava por tmp + rename: um crash no meio da escrita deixa o
// registro anterior ou nenhum, nunca um JSON pela metade.
func writeRecord(runDir string, r procRecord) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	path := recordPath(runDir, r.ID)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".rec-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// forgetProcess apaga o registro quando o processo é liberado (Stop, restart,
// Close): só sobra registro de quem o app não chegou a liberar.
func forgetProcess(runDir, id string) {
	if runDir == "" {
		return
	}
	_ = os.Remove(recordPath(runDir, id))
}

// readProcInfo lê do kernel o que o shouldReap compara. Zumbi conta como
// morto, como em exited.
func readProcInfo(pid int) procInfo {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kp.Proc.P_pid != int32(pid) || kp.Proc.P_stat == sZomb {
		return procInfo{}
	}
	info := procInfo{
		alive:     true,
		startSec:  kp.Proc.P_starttime.Sec,
		startUsec: int64(kp.Proc.P_starttime.Usec),
		pgid:      int(kp.Eproc.Pgid),
		ppid:      int(kp.Eproc.Ppid),
	}
	if buf, err := unix.SysctlRaw("kern.procargs2", pid); err == nil {
		info.exe = parseProcargs(buf)
	}
	return info
}

// orphan é um registro conferido: o grupo dele vai ser encerrado.
type orphan struct {
	rec  procRecord
	path string
}

// ReapOrphans encerra os grupos de serviços que uma execução anterior do app
// iniciou e não liberou (crash, kill -9). Roda só na instância primária,
// antes de qualquer serviço desta execução subir. Todo registro lido é
// apagado: o que bate com um órfão vivo é encerrado (SIGTERM ao grupo, prazo
// de stopTimeout, SIGKILL); o que não bate é apagado sem matar nada. Devolve
// os ids encerrados.
func ReapOrphans(runDir string, logger *slog.Logger) []string {
	if runDir == "" {
		return nil
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	dir := filepath.Join(runDir, "procs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("ler registros de processos", "dir", dir, "err", err)
		}
		return nil
	}
	var targets []orphan
	for _, de := range entries {
		path := filepath.Join(dir, de.Name())
		switch {
		case de.IsDir():
			continue
		case strings.HasSuffix(de.Name(), ".tmp"):
			// Sobra de uma gravação interrompida pelo crash.
			_ = os.Remove(path)
			continue
		case !strings.HasSuffix(de.Name(), ".json"):
			continue
		}
		var r procRecord
		data, err := os.ReadFile(path)
		if err == nil {
			err = json.Unmarshal(data, &r)
		}
		if err != nil {
			logger.Warn("registro de processo ilegível, descartado", "path", path, "err", err)
			_ = os.Remove(path)
			continue
		}
		info := readProcInfo(r.PID)
		if ok, why := shouldReap(r, info); !ok {
			// Processo já morto é o caso comum depois de um crash (o serviço
			// caiu junto); o resto merece Warn porque o pid está com outro
			// dono — possivelmente um `brew services` do usuário.
			level := slog.LevelWarn
			if !info.alive {
				level = slog.LevelInfo
			}
			logger.Log(context.Background(), level, "registro de processo descartado sem encerrar", "id", r.ID, "pid", r.PID, "exe", r.Exe, "motivo", why)
			_ = os.Remove(path)
			continue
		}
		targets = append(targets, orphan{rec: r, path: path})
	}
	if len(targets) == 0 {
		return nil
	}

	// SIGTERM em todos antes de esperar: o prazo vale para o conjunto, não
	// stopTimeout por órfão.
	for _, o := range targets {
		logger.Info("encerrando serviço órfão de uma execução anterior", "id", o.rec.ID, "pid", o.rec.PID, "exe", o.rec.Exe)
		_ = syscall.Kill(-o.rec.PGID, syscall.SIGTERM)
	}
	exitedOK := make([]bool, len(targets))
	var wg sync.WaitGroup
	for i, o := range targets {
		wg.Go(func() { exitedOK[i] = waitExit(o.rec.PID, stopTimeout) })
	}
	wg.Wait()

	killed := make([]string, 0, len(targets))
	for i, o := range targets {
		if !exitedOK[i] {
			// Um mysqld com InnoDB grande pode passar do prazo; o SIGKILL
			// custa um crash recovery no próximo start, por isso o Warn.
			logger.Warn("órfão não encerrou com SIGTERM no prazo, enviando SIGKILL", "id", o.rec.ID, "pid", o.rec.PID, "prazo", stopTimeout.String())
		}
		// SIGKILL ao grupo também quando o líder saiu, como o
		// closeProcessHandle faz no Stop: alcança um membro que ignorou o
		// SIGTERM. O grupo ainda é o conferido: um pgid não é reaproveitado
		// enquanto o grupo tem membros, e sem membros o kill dá ESRCH. O
		// XNU aloca pids em sequência até o PID_MAX, então um processo novo
		// liderando um grupo com esse número em milissegundos não ocorre.
		_ = syscall.Kill(-o.rec.PGID, syscall.SIGKILL)
		if !exitedOK[i] && !waitExit(o.rec.PID, time.Second) {
			logger.Warn("órfão não encerrou após SIGKILL", "id", o.rec.ID, "pid", o.rec.PID)
		}
		_ = os.Remove(o.path)
		killed = append(killed, o.rec.ID)
	}
	return killed
}
