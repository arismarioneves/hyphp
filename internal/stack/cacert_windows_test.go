package stack

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
	"hyphp/internal/pkgmgr"
	"hyphp/internal/runtime"
)

var comPHP = []runtime.Installed{{Kind: runtime.PHP, Version: "8.3.35"}}

// pilhaComCAs monta um Stack cujo catálogo tem o pacote de CAs de 2026-09-25.
func pilhaComCAs(t *testing.T, fetch func(context.Context, pkgmgr.Package, string) error) *Stack {
	t.Helper()
	t.Setenv(paths.EnvRoot, t.TempDir())
	cat := pkgmgr.Catalog{Packages: []pkgmgr.Package{{ID: "cacert-2026-09-25", Kind: pkgmgr.KindCACert, Version: "2026-09-25"}}}
	return &Stack{d: Deps{
		Catalog: func() pkgmgr.Catalog { return cat },
		Fetch:   fetch,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}
}

func gravar(t *testing.T, path, conteudo string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}
}

func lerBundle(t *testing.T, s *Stack) string {
	t.Helper()
	path, warns := s.syncCACert(comPHP)
	if len(warns) != 0 || path != filepath.Join(paths.Etc(), "ssl", "cacert.pem") {
		t.Fatalf("syncCACert = %q, %+v", path, warns)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// esperarDownload espera o download em segundo plano terminar: o próximo
// Reconcile só tenta de novo depois dele.
func esperarDownload(t *testing.T, s *Stack) {
	t.Helper()
	for range 500 {
		if !s.cacertFetching.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("o download do pacote de CAs não terminou")
}

// O bundle é o pacote da Mozilla seguido da CA local: o PHP confia nos sites
// da internet e nos .test em HTTPS. Sem a CA local (certificado ainda não
// instalado), é só o da Mozilla, e ele muda quando ela aparece.
func TestBundleDeCAs(t *testing.T) {
	s := pilhaComCAs(t, nil)
	caroot := t.TempDir()
	s.d.Mkcert = netcfg.Mkcert{Exe: "mkcert.exe", CARoot: caroot}
	gravar(t, filepath.Join(paths.Var(), "cacert", "cacert-2026-09-25.pem"), "MOZILLA\n")

	if got := lerBundle(t, s); got != "MOZILLA\n" {
		t.Errorf("sem a CA local: %q", got)
	}
	gravar(t, filepath.Join(caroot, "rootCA.pem"), "CA LOCAL\n")
	if got := lerBundle(t, s); got != "MOZILLA\n\nCA LOCAL\n" {
		t.Errorf("com a CA local: %q", got)
	}
}

// Sem pacote baixado: aviso, nenhum caminho para o php.ini e um download só,
// mesmo com Reconciles seguidos. Um download que falhou é tentado de novo no
// Reconcile seguinte.
func TestSemPacoteDeCAsAvisaEBaixa(t *testing.T) {
	pedidos := make(chan string, 4)
	liberar := make(chan struct{})
	s := pilhaComCAs(t, func(_ context.Context, _ pkgmgr.Package, dest string) error {
		pedidos <- dest
		<-liberar
		return errors.New("curl.se fora do ar")
	})
	for range 2 {
		if path, warns := s.syncCACert(comPHP); path != "" || len(warns) != 1 || warns[0].Code != "cacert-missing" {
			t.Fatalf("syncCACert = %q, %+v", path, warns)
		}
	}
	close(liberar)
	esperarDownload(t, s)
	if len(pedidos) != 1 {
		t.Fatalf("%d downloads com um em voo; quer 1", len(pedidos))
	}
	if dest := <-pedidos; dest != filepath.Join(paths.Var(), "cacert", "cacert-2026-09-25.pem") {
		t.Errorf("download para %s", dest)
	}
	s.syncCACert(comPHP)
	esperarDownload(t, s)
	if len(pedidos) != 1 {
		t.Error("o download que falhou não foi tentado de novo")
	}
}

// Versão nova no catálogo: a anterior segue no bundle até a nova chegar, sem
// o php.ini perder as linhas no meio do caminho. Quando a nova chega, a
// anterior sai de var/cacert.
func TestPacoteAnteriorValeAteONovoChegar(t *testing.T) {
	s := pilhaComCAs(t, func(context.Context, pkgmgr.Package, string) error { return errors.New("ainda não") })
	antigo := filepath.Join(paths.Var(), "cacert", "cacert-2026-06-01.pem")
	gravar(t, antigo, "ANTIGO\n")
	if got := lerBundle(t, s); got != "ANTIGO\n" {
		t.Errorf("bundle = %q, quer o pacote anterior", got)
	}
	esperarDownload(t, s)

	gravar(t, filepath.Join(paths.Var(), "cacert", "cacert-2026-09-25.pem"), "NOVO\n")
	if got := lerBundle(t, s); got != "NOVO\n" {
		t.Errorf("bundle = %q, quer o pacote novo", got)
	}
	if _, err := os.Stat(antigo); !os.IsNotExist(err) {
		t.Error("o pacote anterior ficou em var/cacert")
	}
}

// Sem PHP instalado não há o que configurar: nem aviso, nem download.
func TestSemPHPSemCAs(t *testing.T) {
	s := pilhaComCAs(t, func(context.Context, pkgmgr.Package, string) error {
		t.Error("download sem PHP instalado")
		return nil
	})
	if path, warns := s.syncCACert(nil); path != "" || len(warns) != 0 {
		t.Errorf("syncCACert = %q, %+v", path, warns)
	}
}
