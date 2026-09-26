package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// No build de produção (-H windowsgui) não há console e o stderr é um handle
// inválido. Com io.MultiWriter(os.Stderr, arquivo) o primeiro Write falhava e
// o arquivo nunca recebia nada: log/hyphp.log ficava com 0 bytes para sempre.
func TestLogChegaAoArquivoSemStderrUtilizavel(t *testing.T) {
	stderr, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	stderr.Close() // qualquer Write/Stat agora falha, como o handle nulo da GUI

	var arquivo bytes.Buffer
	slog.New(slog.NewTextHandler(logOutput(stderr, &arquivo), nil)).Info("hyphp iniciando")

	if !strings.Contains(arquivo.String(), "hyphp iniciando") {
		t.Fatalf("log não chegou ao arquivo: %q", arquivo.String())
	}
}

func TestLogVaiParaStderrQuandoHaConsole(t *testing.T) {
	stderr, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()

	var arquivo bytes.Buffer
	slog.New(slog.NewTextHandler(logOutput(stderr, &arquivo), nil)).Info("oi")

	raw, _ := os.ReadFile(stderr.Name())
	if !strings.Contains(string(raw), "oi") || !strings.Contains(arquivo.String(), "oi") {
		t.Errorf("stderr=%q arquivo=%q", raw, arquivo.String())
	}
}
