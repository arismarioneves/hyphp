package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

// O detector precisa achar a versão sem executar nada: phpMyAdmin é código,
// não binário, e abrir um processo por varredura só para ler uma versão sairia
// caro em toda tela de runtimes.
func TestDetectPhpMyAdmin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.php"), []byte("<?php"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("phpMyAdmin 5.2.3\n=====\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	inst, ok := detectPhpMyAdmin(dir)
	if !ok {
		t.Fatal("não detectou")
	}
	if inst.Version != "5.2.3" {
		t.Errorf("Version = %q, quero 5.2.3", inst.Version)
	}
	if inst.Kind != PhpMyAdmin {
		t.Errorf("Kind = %q", inst.Kind)
	}
}

// O README real da release 5.2.3 não escreve a versão colada ao nome: o
// cabeçalho é "phpMyAdmin - Readme" e a versão vem na linha "Version 5.2.3".
// Este é o formato que chega do download, e o detector tem de lê-lo.
func TestDetectPhpMyAdminReadmeDaRelease(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.php"), "<?php")
	mustWrite(t, filepath.Join(dir, "README"), "phpMyAdmin - Readme\n===================\n\nVersion 5.2.3\n\nA web interface for MySQL and MariaDB.\n")

	inst, ok := detectPhpMyAdmin(dir)
	if !ok {
		t.Fatal("não detectou o README da release")
	}
	if inst.Version != "5.2.3" {
		t.Errorf("Version = %q, quero 5.2.3", inst.Version)
	}
}

// Diretório sem index.php não é phpMyAdmin: evita que uma pasta qualquer em
// bin/phpmyadmin/ vire uma instalação fantasma na UI.
func TestDetectPhpMyAdminSemIndex(t *testing.T) {
	if _, ok := detectPhpMyAdmin(t.TempDir()); ok {
		t.Error("detectou diretório vazio")
	}
}

// Scan precisa enxergar o phpMyAdmin como kind versionado (bin/phpmyadmin/<id>/),
// que é onde o pkgmgr extrai o zip: sem isso a ferramenta baixada não aparece.
func TestScanEncontraPhpMyAdmin(t *testing.T) {
	bin := t.TempDir()
	dir := filepath.Join(bin, "phpmyadmin", "phpMyAdmin-5.2.3-all-languages")
	mustWrite(t, filepath.Join(dir, "index.php"), "<?php")
	mustWrite(t, filepath.Join(dir, "README"), "phpMyAdmin 5.2.3\n")
	mustMkdir(t, filepath.Join(bin, "phpmyadmin", "sobra")) // pasta sem index.php

	list, err := Scan(bin)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("Scan = %+v, quero 1 entrada", list)
	}
	if list[0].Kind != PhpMyAdmin || list[0].Version != "5.2.3" || list[0].Dir != dir {
		t.Errorf("Scan[0] = %+v", list[0])
	}
}
