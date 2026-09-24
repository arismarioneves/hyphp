package services

import (
	"os"
	"path/filepath"
	"testing"

	"hyphp/internal/runtime"
)

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
