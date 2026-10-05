package supervisor

import (
	"bytes"
	"encoding/binary"
	"net/url"
	"path/filepath"
)

// A limpeza de órfãos é por SO (orphans_windows.go, orphans_darwin.go), com o
// mesmo trio de funções:
//
//	func recordProcess(runDir string, spec Spec, cmd *exec.Cmd, h procHandle) error
//	func forgetProcess(runDir, id string)
//	func ReapOrphans(runDir string, logger *slog.Logger) []string
//
// Este arquivo guarda a parte sem syscall (formato do registro, leitura do
// kern.procargs2 e a decisão de matar) sem build tag, para os testes dela
// rodarem também no Windows.

// procRecord é o registro de um serviço iniciado, gravado em
// <runDir>/procs/<id escapado>.json. Pid sozinho não identifica um processo
// (o kernel recicla pids); pid + horário de início + pgid + executável sim.
type procRecord struct {
	ID        string `json:"id"`
	PID       int    `json:"pid"`
	PGID      int    `json:"pgid"`
	Exe       string `json:"exe"`
	StartSec  int64  `json:"startSec"`
	StartUsec int64  `json:"startUsec"`
	OwnerPID  int    `json:"ownerPid"`
}

// procInfo é o retrato de um pid vivo lido do kernel. alive é false quando o
// pid não existe ou é zumbi; exe fica vazio se o kern.procargs2 não pôde ser
// lido ou veio truncado.
type procInfo struct {
	alive     bool
	startSec  int64
	startUsec int64
	pgid      int
	ppid      int
	exe       string
}

// recordPath escapa o id porque ele tem ':' e pode ter '/' ("php:8.3",
// "proc:site/fila"); com o PathEscape cada id vira um único nome de arquivo.
func recordPath(runDir, id string) string {
	return filepath.Join(runDir, "procs", url.PathEscape(id)+".json")
}

// parseProcargs extrai o caminho executado do buffer do kern.procargs2:
// argc (int32 na ordem do host), o exec path terminado em NUL, NULs de
// alinhamento e depois o argv. O exec path vem antes do argv justamente
// porque o argv pode ser reescrito pelo próprio processo (o php-fpm troca o
// título para "php-fpm: master process (...)"). Buffer curto ou sem o NUL
// final devolve "": sem o caminho inteiro não há como confirmar o processo.
func parseProcargs(buf []byte) string {
	if len(buf) < 4 {
		return ""
	}
	if int32(binary.NativeEndian.Uint32(buf)) < 0 {
		return ""
	}
	rest := buf[4:]
	end := bytes.IndexByte(rest, 0)
	if end <= 0 {
		return ""
	}
	return string(rest[:end])
}

// sameExecutable compara o executável registrado com o do processo: cru e,
// se diferir, pelo caminho resolvido. O Homebrew expõe opt/<fórmula> como
// symlink para o Cellar, então o mesmo binário pode aparecer pelos dois
// caminhos. Caminho que não resolve é comparado cru (nunca vira curinga).
func sameExecutable(recorded, actual string) bool {
	if recorded == "" || actual == "" {
		return false
	}
	if recorded == actual {
		return true
	}
	return resolvePath(recorded) == resolvePath(actual)
}

func resolvePath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// shouldReap decide se o grupo do registro é um órfão nosso. Só mata se TUDO
// bater (spec §3): o pid vivo com o mesmo horário de início (pid reciclado
// tem outro), o mesmo pgid, o pai é o launchd (ppid 1: o HyPHP que o iniciou
// morreu; com o dono vivo o pai seria ele) e o executável é o registrado. Não
// há atalho por porta ou por "binário do Homebrew": o `brew services` do
// usuário roda os mesmos binários, também com ppid 1. Devolve o motivo quando
// não mata.
func shouldReap(r procRecord, info procInfo) (bool, string) {
	// pgid <= 1 viraria kill(-1) ou kill(0): todos os processos do usuário ou
	// o grupo do próprio app. E o registro sempre nasce com pgid == pid
	// (Setpgid); outro valor é registro corrompido, e o sinal iria a um grupo
	// cujo líder não foi conferido.
	switch {
	case r.PID <= 1 || r.PGID <= 1 || r.PGID != r.PID:
		return false, "registro com pid/pgid inválido"
	case r.Exe == "":
		return false, "registro sem executável"
	case !info.alive:
		return false, "processo não existe mais"
	case info.startSec != r.StartSec || info.startUsec != r.StartUsec:
		return false, "horário de início diferente (pid reutilizado)"
	case info.pgid != r.PGID:
		return false, "grupo de processos diferente"
	case info.ppid != 1:
		return false, "processo ainda tem pai vivo"
	case !sameExecutable(r.Exe, info.exe):
		return false, "executável diferente do registrado"
	}
	return true, ""
}
