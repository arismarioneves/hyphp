//go:build darwin

package brew

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"hyphp/internal/pkgmgr"
)

// busy garante uma operação do brew por vez no processo (ver ErrBusy).
var busy sync.Mutex

// termDelay é o prazo entre o SIGINT do cancelamento e o SIGTERM: o brew trata
// Interrupt e limpa o keg pela metade, o que leva alguns segundos.
const termDelay = 10 * time.Second

// tailLines é quantas linhas finais da saída vão no erro de uma instalação.
const tailLines = 20

// armPrefix é o prefixo padrão do Homebrew no Apple Silicon.
const armPrefix = "/opt/homebrew"

// Locate acha o brew: HOMEBREW_PREFIX (definido pelo shellenv do usuário),
// depois o prefixo padrão do Apple Silicon e, por último, o PATH. App aberto
// pelo Finder não herda o PATH do shell, por isso o PATH é o último recurso.
func Locate(ctx context.Context) (Brew, error) {
	if p := os.Getenv("HOMEBREW_PREFIX"); p != "" {
		if exe := filepath.Join(p, "bin", "brew"); isExecutable(exe) {
			return Brew{Exe: exe, Prefix: p}, nil
		}
	}
	if exe := filepath.Join(armPrefix, "bin", "brew"); isExecutable(exe) {
		return Brew{Exe: exe, Prefix: armPrefix}, nil
	}
	exe, err := exec.LookPath("brew")
	if err != nil {
		return Brew{}, ErrNotFound
	}
	out, err := exec.CommandContext(ctx, exe, "--prefix").Output()
	if err != nil {
		return Brew{}, fmt.Errorf("brew: %s --prefix: %w", exe, err)
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return Brew{}, fmt.Errorf("brew: %s --prefix não devolveu caminho", exe)
	}
	return Brew{Exe: exe, Prefix: p}, nil
}

func isExecutable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0
}

// env herda o ambiente do app (o brew precisa de HOME, USER, TMPDIR) e fixa o
// resto: PATH mínimo com o prefixo (app de GUI não tem o PATH do shell), sem
// dicas e sem cor na saída que vira mensagem, e sem o check de dependentes,
// que faria uma instalação de PHP atualizar fórmulas alheias do usuário. Em
// chave repetida, o exec usa a última ocorrência.
func (b Brew) env() []string {
	return append(os.Environ(),
		"PATH="+b.Prefix+"/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"HOMEBREW_NO_ENV_HINTS=1",
		"HOMEBREW_NO_COLOR=1",
		"HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1",
	)
}

// Install roda `brew install <Name>` e repassa cada linha de saída como um
// Progress (Total -1: o brew não informa bytes). As fases terminais (done,
// error, canceled) ficam com quem chama, que ainda detecta o keg instalado.
// Cancelar o ctx devolve ctx.Err(); falha devolve as últimas linhas da saída.
func (b Brew) Install(ctx context.Context, f Formula, on func(pkgmgr.Progress)) error {
	if !busy.TryLock() {
		return ErrBusy
	}
	defer busy.Unlock()

	hadKeg := b.kegInOpt(f)
	// stdout e stderr no mesmo pipe preservam a ordem das linhas.
	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("brew: %w", err)
	}
	defer pr.Close()

	cmd := exec.CommandContext(ctx, b.Exe, "install", f.Name)
	cmd.Env = b.env()
	cmd.Stdout, cmd.Stderr = pw, pw
	// Grupo próprio: o brew dispara curl, tar e scripts Ruby; o sinal precisa
	// chegar a todos, não só ao processo do bash que abre o brew.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	exited := make(chan struct{})
	cmd.Cancel = func() error {
		pgid := -cmd.Process.Pid
		go func() {
			select {
			case <-exited:
			case <-time.After(termDelay):
				_ = syscall.Kill(pgid, syscall.SIGTERM)
			}
		}()
		return syscall.Kill(pgid, syscall.SIGINT)
	}
	err = cmd.Start()
	// O pai fecha sua ponta de escrita para a leitura ver EOF quando o brew sair.
	pw.Close()
	if err != nil {
		return fmt.Errorf("brew: iniciar %s install: %w", b.Exe, err)
	}

	phase := pkgmgr.PhaseDownload
	tail := make([]string, 0, tailLines)
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\t ")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if p := phaseOf(line); p != "" {
			phase = p
		}
		if len(tail) == tailLines {
			copy(tail, tail[1:])
			tail = tail[:tailLines-1]
		}
		tail = append(tail, line)
		on(pkgmgr.Progress{Phase: phase, Total: -1, Message: line})
	}
	if sc.Err() != nil {
		// Linha acima do limite: drena o resto para o brew não travar com o
		// pipe cheio.
		_, _ = io.Copy(io.Discard, pr)
	}
	err = cmd.Wait()
	close(exited)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		// O brew sai com 1 quando só o link em <prefix>/bin falha por conflito
		// (mariadb@11.8 e mysql@8.4 trazem os dois bin/mysql). O keg já está
		// instalado e o opt/ já aponta para ele, porque o link do opt vem
		// antes. O HyPHP usa só o opt/, com as próprias configs, e nunca roda
		// brew link: o que falhou depois disso (link, post_install) não o
		// atinge, e o keg que apareceu no opt/ com esta instalação é sucesso.
		if !hadKeg && b.kegInOpt(f) {
			return nil
		}
		return fmt.Errorf("brew install %s: %w\n%s", f.Name, err, strings.Join(tail, "\n"))
	}
	return nil
}

// kegInOpt diz se <prefix>/opt/<nome curto> aponta para uma pasta de keg.
func (b Brew) kegInOpt(f Formula) bool {
	st, err := os.Stat(filepath.Join(b.Prefix, "opt", f.Short()))
	return err == nil && st.IsDir()
}

// Uninstall roda `brew uninstall <nome curto>`. O texto do brew vai no erro
// como está (ex.: recusa porque outra fórmula depende desta).
func (b Brew) Uninstall(ctx context.Context, f Formula) error {
	if !busy.TryLock() {
		return ErrBusy
	}
	defer busy.Unlock()

	cmd := exec.CommandContext(ctx, b.Exe, "uninstall", f.Short())
	cmd.Env = b.env()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("brew uninstall %s: %w\n%s", f.Short(), err, strings.TrimSpace(string(out)))
	}
	return nil
}
