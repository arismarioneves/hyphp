package stack

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"hyphp/internal/paths"
	"hyphp/internal/render"
	"hyphp/internal/runtime"
	"hyphp/internal/state"
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
	content := render.RenderMyIni(port, inst.Dir, MySQLDataDir(varDir), logDir)
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
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
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
	list := runtime.ByKind(rts, runtime.MySQL)
	if len(list) == 0 {
		return rts, false, nil
	}
	inst := list[0]
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
			Message: fmt.Sprintf("MySQL não foi inicializado e não será iniciado: %v", err),
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
