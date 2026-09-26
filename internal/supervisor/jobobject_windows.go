package supervisor

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// newKillOnCloseJob cria um Job Object com JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE:
// quando o último handle do job for fechado, o kernel mata todo processo
// associado. O chamador é dono do handle.
//
// JOB_OBJECT_LIMIT_BREAKAWAY_OK existe para um único processo: o que aplica o
// update. Ele precisa sobreviver ao app, porque espera o app morrer para rodar
// o instalador; dentro do job, o kernel o mataria nesse mesmo instante. A flag
// só libera quem pede CREATE_BREAKAWAY_FROM_JOB ao nascer; os serviços não
// pedem, continuam presos, e a garantia de zero órfãos (spec §7.1) se mantém.
func newKillOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("CreateJobObject: %w", err)
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		windows.CloseHandle(job)
		return 0, fmt.Errorf("SetInformationJobObject: %w", err)
	}
	return job, nil
}

// assignPID coloca o processo pid dentro de job. AssignProcessToJobObject exige
// PROCESS_SET_QUOTA | PROCESS_TERMINATE no handle do processo.
func assignPID(job windows.Handle, pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("OpenProcess(%d): %w", pid, err)
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		return fmt.Errorf("AssignProcessToJobObject(%d): %w", pid, err)
	}
	return nil
}

var (
	selfJobOnce sync.Once
	selfJob     windows.Handle
	selfJobErr  error
)

// AttachSelfToKillOnCloseJob cria o job global e associa o PROCESSO ATUAL a
// ele. Todo filho criado depois herda o job automaticamente — por isso não
// há janela de corrida nem necessidade de CREATE_SUSPENDED (spec §7.1).
//
// O handle devolvido NUNCA deve ser fechado enquanto o processo vive: fechar
// o último handle mata o próprio hyphp.exe. Quando o processo termina (mesmo
// por taskkill /F) o kernel fecha o handle e mata o que restou da árvore.
// Idempotente: chamadas seguintes devolvem o mesmo handle.
func AttachSelfToKillOnCloseJob() (windows.Handle, error) {
	selfJobOnce.Do(func() {
		job, err := newKillOnCloseJob()
		if err != nil {
			selfJobErr = err
			return
		}
		if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
			windows.CloseHandle(job)
			selfJobErr = fmt.Errorf("AssignProcessToJobObject(self): %w", err)
			return
		}
		selfJob = job
	})
	return selfJob, selfJobErr
}
