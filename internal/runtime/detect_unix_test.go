//go:build darwin || linux

package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mailpitLento escreve um bin/mailpit que demora atrasoPrimeira na primeira
// execução (o binário frio, carregado do disco) e responde na hora depois.
func mailpitLento(t *testing.T, atrasoPrimeira string) string {
	t.Helper()
	dir := t.TempDir()
	marca := filepath.Join(dir, "ja-rodou")
	script := "#!/bin/sh\nif [ ! -e '" + marca + "' ]; then : > '" + marca + "'; exec sleep " + atrasoPrimeira + "; fi\necho 'mailpit v1.31.6'\n"
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "mailpit"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func prazos(t *testing.T, primeiro, segundo time.Duration) {
	t.Helper()
	d, s := detectTimeout, slowDetectTimeout
	detectTimeout, slowDetectTimeout = primeiro, segundo
	t.Cleanup(func() { detectTimeout, slowDetectTimeout = d, s })
}

// Um binário que passa do prazo só na primeira execução continua na lista:
// sem a segunda tentativa, o PHP de uma máquina lenta sumia logo ao abrir o app.
func TestDeteccaoTentaDeNovoQuandoOBinarioPassaDoPrazo(t *testing.T) {
	prazos(t, 200*time.Millisecond, 5*time.Second)
	inst, err := Detect(Mailpit, mailpitLento(t, "3"))
	if err != nil || inst.Version != "1.31.6" {
		t.Fatalf("Detect = %+v, %v", inst, err)
	}
}

// Um binário que trava sempre não segura a varredura além da segunda tentativa.
func TestDeteccaoDesisteDoBinarioQueTravaSempre(t *testing.T) {
	prazos(t, 100*time.Millisecond, 300*time.Millisecond)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "mailpit"), []byte("#!/bin/sh\nexec sleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	inicio := time.Now()
	if _, err := Detect(Mailpit, dir); err == nil {
		t.Fatal("binário travado foi detectado")
	}
	if d := time.Since(inicio); d > 2*time.Second {
		t.Fatalf("a detecção levou %s", d)
	}
}
