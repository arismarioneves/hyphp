package services

import (
	"os"
	"path/filepath"
	"testing"

	"hyphp/internal/pkgmgr"
	"hyphp/internal/runtime"
)

// No Windows não há Homebrew: Supported=false é o que esconde o banner e os
// rótulos de brew na UI.
func TestHomebrewNaoSuportadoNoWindows(t *testing.T) {
	r := NewRuntimesService(RuntimesDeps{BinDir: t.TempDir()})
	if got := r.Homebrew(); got != (BrewStatus{}) {
		t.Fatalf("Homebrew() = %+v; quer Supported=false e o resto vazio", got)
	}
}

// Importar aceita tanto a pasta da build quanto uma pasta que contém várias
// builds — que é como o Laragon e o XAMPP guardam (bin/php/php-8.1.10/...).
func TestImportFromCopiaBuildReconhecida(t *testing.T) {
	origem := t.TempDir()
	build := filepath.Join(origem, "php-8.1.10-Win32-vs16-x64")
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(build, "php.exe"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	destino := t.TempDir()
	if err := copyRuntimeTree(build, filepath.Join(destino, "php-8.1.10-Win32-vs16-x64")); err != nil {
		t.Fatalf("copyRuntimeTree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destino, "php-8.1.10-Win32-vs16-x64", "php.exe")); err != nil {
		t.Errorf("php.exe não foi copiado: %v", err)
	}
}

// A origem não pode ser consumida: o usuário pode estar importando de uma
// instalação que ainda usa.
func TestImportFromPreservaOrigem(t *testing.T) {
	origem := t.TempDir()
	if err := os.WriteFile(filepath.Join(origem, "php.exe"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	destino := filepath.Join(t.TempDir(), "php-8.1.10")

	if err := copyRuntimeTree(origem, destino); err != nil {
		t.Fatalf("copyRuntimeTree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(origem, "php.exe")); err != nil {
		t.Errorf("origem foi consumida: %v", err)
	}
}

// O nome da pasta de destino decide o que o usuário vê na lista de instalados e
// se duas builds diferentes colidem. Importar do XAMPP entrega uma pasta
// chamada só "php": copiada com esse nome daria bin/php/php, que não diz qual
// versão é e colide com a próxima importação do mesmo tipo.
func TestDestinoImportNomeiaPelaVersaoQuandoPastaTemNomeDoKind(t *testing.T) {
	bin := filepath.Join("C:", "hyphp", "bin")
	casos := []struct {
		nome  string
		inst  runtime.Installed
		quero string
	}{
		{
			nome:  "pasta com nome próprio é preservada",
			inst:  runtime.Installed{Kind: runtime.PHP, Version: "8.1.10", Dir: filepath.Join("C:", "laragon", "bin", "php", "php-8.1.10-Win32-vs16-x64")},
			quero: filepath.Join(bin, "php", "php-8.1.10-Win32-vs16-x64"),
		},
		{
			nome:  "pasta chamada como o kind ganha a versão",
			inst:  runtime.Installed{Kind: runtime.PHP, Version: "8.1.10", Dir: filepath.Join("C:", "xampp", "php")},
			quero: filepath.Join(bin, "php", "php-8.1.10"),
		},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := destinoImport(bin, c.inst); got != c.quero {
				t.Errorf("destinoImport = %q, quero %q", got, c.quero)
			}
		})
	}
}

// O pacote de CAs vem no catálogo, mas não é runtime: não aparece para
// instalar nem pode ser pedido pela tela.
func TestCACertForaDaTelaRuntimes(t *testing.T) {
	cat := pkgmgr.Catalog{Packages: []pkgmgr.Package{
		{ID: "php-8.3.35-nts-vs16-x64", Kind: runtime.PHP, Version: "8.3.35"},
		{ID: "cacert-2026-09-25", Kind: pkgmgr.KindCACert, Version: "2026-09-25"},
	}}
	r := NewRuntimesService(RuntimesDeps{BinDir: t.TempDir(), Catalog: func() pkgmgr.Catalog { return cat }})
	got := r.available(nil)
	if len(got) != 1 || got[0].Kind != runtime.PHP {
		t.Errorf("available = %+v, quer só o PHP", got)
	}
	if _, err := r.installer("cacert-2026-09-25"); err == nil {
		t.Error("o pacote de CAs foi aceito como instalação")
	}
}

// O catálogo remoto troca com o app aberto: a tela e a instalação passam a
// usar o catálogo novo sem reiniciar. Um link corrigido só chega ao usuário
// assim.
func TestRuntimesUsamOCatalogoEmUso(t *testing.T) {
	antigo := pkgmgr.Package{ID: "apache-2.4.68-vs18-x64", Kind: runtime.Apache, Version: "2.4.68"}
	novo := pkgmgr.Package{ID: "apache-2.4.69-vs18-x64", Kind: runtime.Apache, Version: "2.4.69"}
	cat := pkgmgr.Catalog{Packages: []pkgmgr.Package{antigo}}
	r := NewRuntimesService(RuntimesDeps{BinDir: t.TempDir(), Catalog: func() pkgmgr.Catalog { return cat }})

	cat = pkgmgr.Catalog{Packages: []pkgmgr.Package{novo}}
	if got := r.available(nil); len(got) != 1 || got[0].ID != novo.ID {
		t.Errorf("available = %+v, quer o pacote do catálogo novo", got)
	}
	if _, err := r.installer(novo.ID); err != nil {
		t.Errorf("pacote do catálogo novo recusado: %v", err)
	}
	if _, err := r.installer(antigo.ID); err == nil {
		t.Error("pacote que saiu do catálogo ainda instala")
	}
}
