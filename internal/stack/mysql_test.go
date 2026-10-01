package stack

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"hyphp/internal/runtime"
	"hyphp/internal/sysproc"
)

// mysqlEm devolve um MySQL "instalado" em dir e as pastas etc/var/log de um
// HyPHP temporário.
func mysqlEm(t *testing.T, dir string) (inst runtime.Installed, etcDir, varDir, logDir string) {
	t.Helper()
	root := t.TempDir()
	inst = runtime.Installed{
		Kind: runtime.MySQL, Version: "8.4.11", Major: "8.4.11", Dir: dir,
		Exe: filepath.Join(dir, "bin", sysproc.ExeName("mysqld")),
	}
	return inst, filepath.Join(root, "etc"), filepath.Join(root, "var"), filepath.Join(root, "log")
}

// Um init morto no meio deixa o mysql.ibd sem o marcador. Adotar esse datadir
// gravava o marcador sobre restos e o mysqld entrava num ciclo de reinícios;
// com o marcador de init em andamento ele é apagado e inicializado de novo.
func TestInitDBDataNaoAdotaInitInterrompido(t *testing.T) {
	// Sem o mysqld: a nova inicialização falha, o que basta para ver que o
	// datadir não foi adotado.
	inst, etcDir, varDir, logDir := mysqlEm(t, t.TempDir())
	data := DataDir(varDir, runtime.MySQL)
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "mysql.ibd"), []byte("pela metade"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(data+initPending, []byte("hyphp\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := InitDBData(t.Context(), inst, 3306, etcDir, varDir, logDir); err == nil {
		t.Fatal("InitDBData adotou o datadir de um init interrompido")
	}
	if _, err := os.Stat(filepath.Join(data, initMarker)); !os.IsNotExist(err) {
		t.Fatalf("marcador de inicializado gravado sobre restos: %v", err)
	}
	if _, err := os.Stat(filepath.Join(data, "mysql.ibd")); !os.IsNotExist(err) {
		t.Fatalf("restos do init interrompido continuam no datadir: %v", err)
	}
	if _, err := os.Stat(data + initPending); err != nil {
		t.Fatalf("o init falhou de novo; o marcador de em andamento tem de ficar: %v", err)
	}
}

// Sem o marcador de em andamento, um datadir com as tabelas do sistema é de
// uma instalação anterior: adota, nunca apaga.
func TestInitDBDataAdotaDatadirExistente(t *testing.T) {
	inst, etcDir, varDir, logDir := mysqlEm(t, t.TempDir())
	data := DataDir(varDir, runtime.MySQL)
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "mysql.ibd"), []byte("dados"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := InitDBData(t.Context(), inst, 3306, etcDir, varDir, logDir); err != nil {
		t.Fatalf("InitDBData: %v", err)
	}
	if _, err := os.Stat(filepath.Join(data, initMarker)); err != nil {
		t.Fatalf("datadir existente não foi adotado: %v", err)
	}
	if _, err := os.Stat(filepath.Join(data, "mysql.ibd")); err != nil {
		t.Fatalf("adoção apagou dados: %v", err)
	}
}

// O ctx do Reconcile vence em 60s, antes do initTimeout; o init não pode
// herdar esse prazo. Um ctx já cancelado mostra isso sem esperar minutos:
// o inicializador tem de rodar mesmo assim.
func TestInitDBDataNaoHerdaPrazoDoChamador(t *testing.T) {
	// Qualquer executável serve de mysqld: o próprio binário de teste recusa
	// as flags e sai na hora, mas só se chegar a ser iniciado.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Nome e bit de execução como o InitDBData procura: mysqld.exe no Windows,
	// mysqld executável no macOS (sem o bit o exec falharia antes de rodar).
	mysqld := filepath.Join(dir, "bin", sysproc.ExeName("mysqld"))
	copyFile(t, self, mysqld)
	if err := os.Chmod(mysqld, 0o755); err != nil {
		t.Fatal(err)
	}
	inst, etcDir, varDir, logDir := mysqlEm(t, dir)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = InitDBData(ctx, inst, 3306, etcDir, varDir, logDir)
	if errors.Is(err, context.Canceled) {
		t.Fatalf("o init herdou o cancelamento do chamador: %v", err)
	}
	if out, rerr := os.ReadFile(filepath.Join(logDir, "mysql-init.log")); rerr != nil || len(out) == 0 {
		t.Fatalf("o inicializador não rodou (log vazio): %v", rerr)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}
