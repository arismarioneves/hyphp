package stack

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"hyphp/internal/i18n"
	"hyphp/internal/mysqlcli"
	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
	"hyphp/internal/project"
	"hyphp/internal/render"
	"hyphp/internal/runtime"
	"hyphp/internal/state"
	"hyphp/internal/supervisor"
)

const (
	// initMarker fica dentro do datadir e só é criado depois de o
	// --initialize-insecure terminar com sucesso.
	initMarker = ".hyphp-initialized"
	// initTimeout cobre o --initialize-insecure: medido em 25s num SSD com o
	// MySQL 8.0.30; num HDD passa de um minuto.
	initTimeout = 180 * time.Second
)

// MySQLDataDir é o datadir do servidor embutido (spec §8).
func MySQLDataDir(varDir string) string { return filepath.Join(varDir, "mysql-data") }

// MyIniPath é o arquivo de opções gerado.
func MyIniPath(etcDir string) string { return filepath.Join(etcDir, "mysql", "my.ini") }

// WriteMyIni grava etc/mysql/my.ini e informa se o conteúdo mudou. "Mudou"
// significa reiniciar o mysqld: sem isso uma troca de porta no state ficaria
// só no arquivo.
func WriteMyIni(inst runtime.Installed, port int, etcDir, varDir, logDir string) (bool, error) {
	dir := filepath.Dir(MyIniPath(etcDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("stack: criar %s: %w", dir, err)
	}
	content := render.RenderMyIni(port, inst.Dir, MySQLDataDir(varDir), logDir, netcfg.IPv6Loopback())
	changed, err := render.WriteFiles(dir, map[string][]byte{"my.ini": content})
	if err != nil {
		return false, fmt.Errorf("stack: gravar my.ini: %w", err)
	}
	return changed, nil
}

// InitMySQLData garante um datadir utilizável em <varDir>/mysql-data.
//
// Idempotente, com três situações distintas:
//   - marcador presente → nada a fazer;
//   - datadir com mysql.ibd mas sem marcador (datadir de uma instalação
//     anterior, ou marcador apagado à mão) → adota, só recria o marcador.
//     NUNCA apaga dados que o MySQL já inicializou;
//   - datadir ausente, ou com restos de uma inicialização interrompida (sem
//     mysql.ibd) → apaga e roda o --initialize-insecure. O mysqld recusa
//     datadir não vazio, então a limpeza é obrigatória para poder repetir.
//
// A saída vai para <logDir>/mysql-init.log: sem ela, a causa de uma falha
// (porta, permissão, ACL do diretório) fica invisível.
//
// Grava o my.ini antes de tudo porque o --initialize-insecure lê o mesmo
// --defaults-file do start normal; a chamada é idempotente (WriteFiles não
// reescreve conteúdo igual), então repeti-la no ensureMySQL não custa nada.
func InitMySQLData(ctx context.Context, inst runtime.Installed, port int, etcDir, varDir, logDir string) error {
	if _, err := WriteMyIni(inst, port, etcDir, varDir, logDir); err != nil {
		return err
	}
	data := MySQLDataDir(varDir)
	if _, err := os.Stat(filepath.Join(data, initMarker)); err == nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(data, "mysql.ibd")); err == nil {
		return writeMarker(data)
	}
	if err := os.RemoveAll(data); err != nil {
		return fmt.Errorf("stack: limpar datadir %s: %w", data, err)
	}
	if err := os.MkdirAll(filepath.Dir(data), 0o755); err != nil {
		return fmt.Errorf("stack: criar %s: %w", filepath.Dir(data), err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("stack: criar %s: %w", logDir, err)
	}

	ctx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(inst.Dir, "bin", "mysqld.exe"),
		"--defaults-file="+MyIniPath(etcDir), "--initialize-insecure", "--console")
	cmd.Dir = inst.Dir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, runErr := cmd.CombinedOutput()

	logPath := filepath.Join(logDir, "mysql-init.log")
	if err := os.WriteFile(logPath, out, 0o644); err != nil {
		return fmt.Errorf("stack: gravar %s: %w", logPath, err)
	}
	if runErr != nil {
		return fmt.Errorf("stack: mysqld --initialize-insecure falhou (veja %s): %w", logPath, runErr)
	}
	return writeMarker(data)
}

func writeMarker(dataDir string) error {
	path := filepath.Join(dataDir, initMarker)
	if err := os.WriteFile(path, []byte("hyphp\n"), 0o644); err != nil {
		return fmt.Errorf("stack: gravar %s: %w", path, err)
	}
	return nil
}

// ensureMySQLData é o wrapper de instância: resolve os diretórios de runtime.
func (s *Stack) ensureMySQLData(ctx context.Context, inst runtime.Installed, st state.State) error {
	return InitMySQLData(ctx, inst, st.MySQLPort, paths.Etc(), paths.Var(), paths.Log())
}

// ensureMySQL prepara o MySQL antes de desired(): grava o my.ini e inicializa o
// datadir. Falha → warning db-init-failed e o runtime sai da lista, de modo que
// desired() não emite o spec. Um mysqld sem datadir subiria, morreria, e o
// restart automático transformaria isso num loop de processos.
func (s *Stack) ensureMySQL(ctx context.Context, rts []runtime.Installed, st state.State) ([]runtime.Installed, bool, []Warning) {
	inst, ok := runtime.Newest(rts, runtime.MySQL)
	if !ok {
		return rts, false, nil
	}
	changed, err := WriteMyIni(inst, st.MySQLPort, paths.Etc(), paths.Var(), paths.Log())
	if err != nil {
		return dropKind(rts, runtime.MySQL), false, []Warning{{
			Code: "db-init-failed", Message: err.Error(),
		}}
	}
	if err := s.ensureMySQLData(ctx, inst, st); err != nil {
		s.d.Logger.Error("inicializar datadir do MySQL", "err", err)
		return dropKind(rts, runtime.MySQL), changed, []Warning{{
			Code:    "db-init-failed",
			Message: i18n.T("warn.dbInitFailed", err),
		}}
	}
	return rts, changed, nil
}

// dropKind devolve a lista sem os runtimes daquele tipo.
func dropKind(list []runtime.Installed, k runtime.Kind) []runtime.Installed {
	out := make([]runtime.Installed, 0, len(list))
	for _, i := range list {
		if i.Kind != k {
			out = append(out, i)
		}
	}
	return out
}

// dbSyncTimeout cobre esperar o mysql ficar ready + criar todos os databases.
const dbSyncTimeout = 120 * time.Second

type dbRequest struct {
	ProjectID string
	Name      string
}

// syncDatabases cria, em background, os databases declarados nos manifestos.
//
// Não bloqueia o Reconcile: espera o spec mysql ficar ready pela assinatura do
// supervisor e só então roda os CREATE DATABASE. Uma sincronização por vez;
// Reconciles disparados enquanto ela corre são ignorados — a lista de projetos
// que eles veriam é a mesma, e empilhar goroutines esperando o mesmo evento é
// como se acumula trabalho invisível.
func (s *Stack) syncDatabases(rts []runtime.Installed, projs []project.Project) {
	reqs := make([]dbRequest, 0, len(projs))
	for _, p := range projs {
		if p.Database != "" {
			reqs = append(reqs, dbRequest{ProjectID: p.ID, Name: p.Database})
		}
	}
	if len(reqs) == 0 {
		return
	}
	inst, ok := runtime.Newest(rts, runtime.MySQL)
	if !ok {
		return
	}
	s.stateMu.Lock()
	if s.dbSync {
		s.stateMu.Unlock()
		return
	}
	s.dbSync = true
	s.stateMu.Unlock()

	client := mysqlcli.New(inst, s.State().MySQLPort)
	go func() {
		defer func() {
			s.stateMu.Lock()
			s.dbSync = false
			s.stateMu.Unlock()
		}()
		// Contexto próprio: o ctx do Reconcile é cancelado quando ele retorna.
		ctx, cancel := context.WithTimeout(context.Background(), dbSyncTimeout)
		defer cancel()
		if err := s.waitMySQLReady(ctx); err != nil {
			s.addWarnings([]Warning{{
				Code:    "db-create-failed",
				Message: i18n.T("warn.dbCreateNotReady", err),
			}})
			return
		}
		var warns []Warning
		for _, req := range reqs {
			if err := mysqlcli.ValidateName(req.Name); err != nil {
				// Nome inválido nunca vira comando: vira aviso.
				warns = append(warns, Warning{
					Code: "db-create-failed", ProjectID: req.ProjectID,
					Message: i18n.T("warn.dbCreateInvalidName", req.Name, req.ProjectID, err),
				})
				continue
			}
			if err := client.Create(ctx, req.Name); err != nil {
				warns = append(warns, Warning{
					Code: "db-create-failed", ProjectID: req.ProjectID,
					Message: i18n.T("warn.dbCreateFailed", req.Name, req.ProjectID, err),
				})
				continue
			}
			s.d.Logger.Info("stack: database garantido", "project", req.ProjectID, "database", req.Name)
		}
		if len(warns) > 0 {
			s.addWarnings(warns)
		}
	}()
}

// waitMySQLReady devolve nil assim que o spec mysql estiver ready.
func (s *Stack) waitMySQLReady(ctx context.Context) error {
	ch, cancel := s.d.Sup.Subscribe()
	defer cancel()
	// Consultar o status DEPOIS de assinar: na ordem inversa, a transição que
	// acontecesse entre as duas chamadas se perderia e a espera iria até o
	// timeout com o banco já no ar.
	if st, ok := s.d.Sup.Status(MySQLSpecID); ok && st.State == supervisor.Ready {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return i18n.Errorf("err.stack.mysqlNotReady", ctx.Err())
		case ev := <-ch:
			if ev.Status.ID != MySQLSpecID {
				continue
			}
			switch ev.Status.State {
			case supervisor.Ready:
				return nil
			case supervisor.Failed:
				return i18n.Errorf("err.stack.mysqlFailed", ev.Status.LastError)
			}
		}
	}
}

// addWarnings acrescenta avisos gerados fora do Reconcile e republica a lista.
// O próximo Reconcile recalcula tudo e descarta estes — é o comportamento
// desejado: são avisos sobre o estado de agora.
func (s *Stack) addWarnings(w []Warning) {
	s.stateMu.Lock()
	s.warnings = append(s.warnings, w...)
	all := append([]Warning(nil), s.warnings...)
	s.stateMu.Unlock()
	s.d.Emit("stack:warnings", all)
}
