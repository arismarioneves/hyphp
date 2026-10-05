package supervisor

import (
	"encoding/binary"
	"path/filepath"
	"testing"
)

// procargs monta um buffer no layout do kern.procargs2: argc, exec path com
// NUL, NULs de alinhamento e o argv.
func procargs(argc int32, exe string, argv ...string) []byte {
	buf := binary.NativeEndian.AppendUint32(nil, uint32(argc))
	buf = append(buf, exe...)
	buf = append(buf, 0, 0, 0, 0)
	for _, a := range argv {
		buf = append(buf, a...)
		buf = append(buf, 0)
	}
	return buf
}

func TestParseProcargsDevolveOExecPathENaoOArgv(t *testing.T) {
	// O php-fpm reescreve o argv[0]; o exec path continua o binário.
	buf := procargs(2, "/opt/homebrew/opt/php@8.3/sbin/php-fpm", "php-fpm: master process (/x/php-fpm.conf)", "-F")
	if got := parseProcargs(buf); got != "/opt/homebrew/opt/php@8.3/sbin/php-fpm" {
		t.Fatalf("parseProcargs = %q", got)
	}
}

func TestParseProcargsBufferTruncadoNaoDevolveCaminho(t *testing.T) {
	inteiro := procargs(1, "/bin/sh", "sh")
	casos := map[string][]byte{
		"vazio":            nil,
		"só parte do argc": inteiro[:3],
		"só o argc":        inteiro[:4],
		"caminho sem NUL":  inteiro[:4+len("/bin/s")],
		"exec path vazio":  procargs(1, "", "sh"),
		"argc negativo":    procargs(-1, "/bin/sh"),
	}
	for nome, buf := range casos {
		if got := parseProcargs(buf); got != "" {
			t.Errorf("%s: parseProcargs = %q, want vazio", nome, got)
		}
	}
}

func TestRecordPathEscapaOIdNumUnicoArquivo(t *testing.T) {
	run := filepath.Join("var", "run")
	got := recordPath(run, "proc:site/fila")
	if filepath.Dir(got) != filepath.Join(run, "procs") {
		t.Fatalf("recordPath = %q, fora de procs/", got)
	}
	if base := filepath.Base(got); base != "proc:site%2Ffila.json" {
		t.Fatalf("nome do registro = %q", base)
	}
}

// orfaoValido é um registro e o retrato do kernel que batem em tudo.
func orfaoValido() (procRecord, procInfo) {
	r := procRecord{ID: "web:apache", PID: 4242, PGID: 4242, Exe: "/opt/homebrew/opt/httpd/bin/httpd", StartSec: 1_700_000_000, StartUsec: 123456, OwnerPID: 999}
	info := procInfo{alive: true, startSec: r.StartSec, startUsec: r.StartUsec, pgid: r.PGID, ppid: 1, exe: r.Exe}
	return r, info
}

func TestShouldReapMataSoQuandoTudoBate(t *testing.T) {
	r, info := orfaoValido()
	if ok, why := shouldReap(r, info); !ok {
		t.Fatalf("órfão que bate em tudo não foi encerrado: %s", why)
	}
}

func TestShouldReapPreservaQuandoAlgoNaoBate(t *testing.T) {
	casos := map[string]func(*procRecord, *procInfo){
		"processo morto":           func(_ *procRecord, i *procInfo) { *i = procInfo{} },
		"segundos do início":       func(_ *procRecord, i *procInfo) { i.startSec++ },
		"microssegundos do início": func(_ *procRecord, i *procInfo) { i.startUsec++ },
		"outro grupo":              func(_ *procRecord, i *procInfo) { i.pgid = 777 },
		// O `brew services` do usuário tem ppid 1 e o mesmo exe; o que o
		// distingue é pid + início. Já um pai vivo é o HyPHP (ou outro dono).
		"pai vivo":              func(_ *procRecord, i *procInfo) { i.ppid = 999 },
		"exe diferente":         func(_ *procRecord, i *procInfo) { i.exe = "/usr/sbin/httpd" },
		"exe ilegível":          func(_ *procRecord, i *procInfo) { i.exe = "" },
		"registro sem exe":      func(r *procRecord, i *procInfo) { r.Exe = ""; i.exe = "" },
		"pgid diferente do pid": func(r *procRecord, i *procInfo) { r.PGID = 4000; i.pgid = 4000 },
		// kill(-1) atingiria todos os processos do usuário; kill(0), o
		// grupo do próprio app.
		"pgid 1": func(r *procRecord, i *procInfo) { r.PID, r.PGID, i.pgid = 1, 1, 1 },
		"pgid 0": func(r *procRecord, i *procInfo) { r.PID, r.PGID, i.pgid = 0, 0, 0 },
	}
	for nome, muda := range casos {
		r, info := orfaoValido()
		muda(&r, &info)
		if ok, _ := shouldReap(r, info); ok {
			t.Errorf("%s: shouldReap mandou matar", nome)
		}
	}
}
