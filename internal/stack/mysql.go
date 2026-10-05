package stack

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	"hyphp/internal/sysproc"
)

const (
	// initMarker fica dentro do datadir e só é criado depois de o
	// --initialize-insecure terminar com sucesso.
	initMarker = ".hyphp-initialized"
	// initPending é o sufixo do arquivo, ao lado do datadir (os inicializadores
	// recusam datadir não vazio), que existe enquanto a inicialização roda.
	initPending = ".hyphp-initializing"
	// initTimeout cobre o --initialize-insecure: medido em 25s num SSD com o
	// MySQL 8.0.30; num HDD passa de um minuto.
	initTimeout = 180 * time.Second
)

// DataDir é o datadir do servidor de banco (spec §8). Cada motor tem o seu:
// o datadir do MySQL 8.4 não abre no MariaDB e vice-versa, e trocar de motor
// não pode apagar nem misturar os dados do outro.
func DataDir(varDir string, k runtime.Kind) string {
	if k == runtime.MariaDB {
		return filepath.Join(varDir, "mariadb-data")
	}
	return filepath.Join(varDir, "mysql-data")
}

// DBRuntime devolve o servidor de banco que roda: o motor de state.DBEngine,
// na versão mais nova instalada. Com DBEngine vazio (state.json anterior à v3,
// ou sem escolha feita) vale o MySQL, e o MariaDB só quando é o único
// instalado — quem só baixou o MariaDB não precisa passar por Configurações.
// Motor escolhido e não instalado devolve ok=false, sem cair no outro: o
// usuário veria os databases de outro servidor sem ter pedido.
func DBRuntime(rts []runtime.Installed, st state.State) (runtime.Installed, bool) {
	switch st.DBEngine {
	case state.DBMariaDB:
		return runtime.Newest(rts, runtime.MariaDB)
	case state.DBMySQL:
		return runtime.Newest(rts, runtime.MySQL)
	}
	if inst, ok := runtime.Newest(rts, runtime.MySQL); ok {
		return inst, true
	}
	return runtime.Newest(rts, runtime.MariaDB)
}

// DBName é o nome do motor para mensagens e para o nome do serviço.
func DBName(k runtime.Kind) string {
	if k == runtime.MariaDB {
		return "MariaDB"
	}
	return "MySQL"
}

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
	content := render.RenderMyIni(port, inst.Dir, DataDir(varDir, inst.Kind), logDir, mysqlSocket(varDir), netcfg.IPv6Loopback(), inst.Kind == runtime.MariaDB)
	changed, err := render.WriteFiles(dir, map[string][]byte{"my.ini": content})
	if err != nil {
		return false, fmt.Errorf("stack: gravar my.ini: %w", err)
	}
	return changed, nil
}

// dbInit descreve como cada motor cria um datadir vazio.
type dbInit struct {
	// system é um arquivo que só existe num datadir já inicializado.
	system string
	// cmd devolve o executável e os argumentos da inicialização.
	cmd func(inst runtime.Installed, etcDir, data string) (string, []string)
	log string
}

var dbInits = map[runtime.Kind]dbInit{
	runtime.MySQL: {
		system: "mysql.ibd",
		// Lê o mesmo --defaults-file do start normal: o datadir sai do my.ini.
		// O mysqld só aceita --defaults-file como primeira opção, e é ele que
		// deixa o my.cnf do sistema (/opt/homebrew/etc no Mac) de fora.
		cmd: func(inst runtime.Installed, etcDir, _ string) (string, []string) {
			args := append([]string{"--defaults-file=" + MyIniPath(etcDir), "--initialize-insecure"}, mysqldExtraArgs...)
			return filepath.Join(inst.Dir, "bin", sysproc.ExeName("mysqld")), args
		},
		log: "mysql-init.log",
	},
	runtime.MariaDB: {
		system: filepath.Join("mysql", "global_priv.frm"),
		// O instalador e os argumentos são por SO (db_windows.go/db_darwin.go).
		cmd: func(inst runtime.Installed, _, data string) (string, []string) {
			return mariadbInitCmd(inst, data)
		},
		log: "mariadb-init.log",
	},
}

// InitDBData garante um datadir utilizável para o motor de inst.
//
// Idempotente, com três situações distintas:
//   - marcador presente → nada a fazer;
//   - datadir com as tabelas do sistema mas sem marcador (datadir de uma
//     instalação anterior, ou marcador apagado à mão) → adota, só recria o
//     marcador. NUNCA apaga dados que o servidor já inicializou;
//   - datadir ausente, ou com restos de uma inicialização interrompida →
//     apaga e inicializa. Os dois motores recusam datadir não vazio, então a
//     limpeza é obrigatória para poder repetir.
//
// "Interrompida" é reconhecida pelo <datadir>.hyphp-initializing, gravado
// antes de rodar o inicializador e apagado só no sucesso. Sem ele a
// adoção valia também para restos: o mysql.ibd nasce no começo do
// --initialize, e um init morto no meio (app fechado, queda de energia)
// virava um datadir "adotado" que o mysqld não consegue abrir, num ciclo de
// reinícios sem aviso.
//
// A saída vai para <logDir>/mysql-init.log ou mariadb-init.log: sem ela, a
// causa de uma falha (porta, permissão, ACL do diretório) fica invisível.
//
// Grava o my.ini antes de tudo porque a inicialização do MySQL lê o mesmo
// --defaults-file do start normal; a chamada é idempotente (WriteFiles não
// reescreve conteúdo igual), então repeti-la no ensureMySQL não custa nada.
func InitDBData(ctx context.Context, inst runtime.Installed, port int, etcDir, varDir, logDir string) error {
	if _, err := WriteMyIni(inst, port, etcDir, varDir, logDir); err != nil {
		return err
	}
	how, ok := dbInits[inst.Kind]
	if !ok {
		return fmt.Errorf("stack: %s não é servidor de banco", inst.Kind)
	}
	data := DataDir(varDir, inst.Kind)
	pending := data + initPending
	if _, err := os.Stat(filepath.Join(data, initMarker)); err == nil {
		return nil
	}
	_, perr := os.Stat(pending)
	interrupted := perr == nil
	if _, err := os.Stat(filepath.Join(data, how.system)); err == nil && !interrupted {
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
	if err := os.WriteFile(pending, []byte("hyphp\n"), 0o644); err != nil {
		return fmt.Errorf("stack: gravar %s: %w", pending, err)
	}

	// O prazo é só o initTimeout: o ctx de quem chama é o do Reconcile (60s),
	// e WithTimeout nunca estende o prazo do pai — num HDD o init era morto
	// antes de terminar.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), initTimeout)
	defer cancel()
	exe, args := how.cmd(inst, etcDir, data)
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = inst.Dir
	sysproc.Hide(cmd)
	out, runErr := cmd.CombinedOutput()

	logPath := filepath.Join(logDir, how.log)
	if err := os.WriteFile(logPath, out, 0o644); err != nil {
		return fmt.Errorf("stack: gravar %s: %w", logPath, err)
	}
	if runErr != nil {
		return fmt.Errorf("stack: %s falhou (veja %s): %w", filepath.Base(exe), logPath, runErr)
	}
	// Nesta ordem: uma queda entre as duas linhas deixa o datadir completo sem
	// nenhum marcador, e a próxima chamada o adota — o certo, porque o init
	// terminou.
	if err := os.Remove(pending); err != nil {
		return fmt.Errorf("stack: apagar %s: %w", pending, err)
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

// ensureMySQL prepara o servidor de banco antes de desired(): grava o my.ini e
// inicializa o datadir do motor escolhido. Falha → warning db-init-failed e o
// runtime sai da lista, de modo que desired() não emite o spec. Um servidor
// sem datadir subiria, morreria, e o restart automático transformaria isso num
// loop de processos.
func (s *Stack) ensureMySQL(ctx context.Context, rts []runtime.Installed, st state.State) ([]runtime.Installed, bool, []Warning) {
	inst, ok := DBRuntime(rts, st)
	if !ok {
		if st.DBEngine == state.DBMariaDB || st.DBEngine == state.DBMySQL {
			name := DBName(runtime.Kind(st.DBEngine))
			return rts, false, []Warning{{Code: "db-engine-missing", Message: i18n.T("warn.dbEngineMissing", name)}}
		}
		return rts, false, nil
	}
	changed, err := WriteMyIni(inst, st.MySQLPort, paths.Etc(), paths.Var(), paths.Log())
	if err != nil {
		return dropKind(rts, inst.Kind), false, []Warning{{
			Code: "db-init-failed", Message: err.Error(),
		}}
	}
	if err := InitDBData(ctx, inst, st.MySQLPort, paths.Etc(), paths.Var(), paths.Log()); err != nil {
		s.d.Logger.Error("inicializar datadir do banco", "motor", inst.Kind, "err", err)
		return dropKind(rts, inst.Kind), changed, []Warning{{
			Code:    "db-init-failed",
			Message: i18n.T("warn.dbInitFailed", DBName(inst.Kind), err),
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
	inst, ok := DBRuntime(rts, s.State())
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
