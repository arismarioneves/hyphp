package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// O nome vai embutido no script de -r e num -d: qualquer caractere fora do
// formato abriria injeção de código PHP ou de outra diretiva.
func TestValidIniName(t *testing.T) {
	for _, n := range []string{"memory_limit", "date.timezone", "opcache.jit_buffer_size", "max_input_vars"} {
		if !ValidIniName(n) {
			t.Errorf("%q devia ser aceito", n)
		}
	}
	for _, n := range []string{"", "a;b", "A", "Memory_limit", "max input", "1abc", "_x", "a'b", "a=b", "a\nb", ".x"} {
		if ValidIniName(n) {
			t.Errorf("%q devia ser recusado", n)
		}
	}
}

func TestPHPIniRecusaNomeInvalidoSemExecutar(t *testing.T) {
	// Exe inexistente: se a validação não viesse antes do exec, o erro seria outro.
	inst := Installed{Kind: PHP, Dir: t.TempDir(), Exe: filepath.Join(t.TempDir(), "nao-existe.exe")}
	if _, err := PHPIniBuiltins(context.Background(), inst, nil, []string{"memory_limit", "a');system('x"}); err == nil {
		t.Fatal("PHPIniBuiltins aceitou nome inválido")
	}
	if _, err := PHPIniKnows(context.Background(), inst, nil, "a;b", "1"); err == nil {
		t.Fatal("PHPIniKnows aceitou nome inválido")
	}
	if _, err := PHPIniKnows(context.Background(), inst, nil, "memory_limit", "1G\nextension=x"); err == nil {
		t.Fatal("PHPIniKnows aceitou valor multilinha")
	}
}

// Contra um PHP real, quando houver um em HYPHP_TEST_PHP (pasta da build).
func TestPHPIniContraPHPReal(t *testing.T) {
	dir := os.Getenv("HYPHP_TEST_PHP")
	if dir == "" {
		t.Skip("HYPHP_TEST_PHP não definido")
	}
	// Detect preenche ExtDir pela regra do SO, como na varredura de verdade.
	inst, err := Detect(PHP, dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	got, err := PHPIniBuiltins(ctx, inst, DefaultExtensions, []string{"max_input_vars", "max_execution_time", "nao_existe.diretiva"})
	if err != nil {
		t.Fatal(err)
	}
	if got["max_input_vars"] != "1000" {
		t.Errorf("max_input_vars builtin = %q, quero 1000", got["max_input_vars"])
	}
	if got["max_execution_time"] != "30" {
		t.Errorf("max_execution_time = %q, quero o builtin 30 (não o 0 da CLI)", got["max_execution_time"])
	}
	if _, ok := got["nao_existe.diretiva"]; ok {
		t.Error("diretiva desconhecida apareceu no mapa")
	}

	if ok, err := PHPIniKnows(ctx, inst, DefaultExtensions, "max_input_vars", "5000"); err != nil || !ok {
		t.Errorf("max_input_vars=5000: ok=%v err=%v", ok, err)
	}
	if ok, err := PHPIniKnows(ctx, inst, DefaultExtensions, "nao_existe.diretiva", "1"); err != nil || ok {
		t.Errorf("diretiva desconhecida: ok=%v err=%v", ok, err)
	}
	// Diretiva de extensão só existe com a DLL carregada; -n sozinho a recusaria.
	if ok, err := PHPIniKnows(ctx, inst, DefaultExtensions, "opcache.memory_consumption", "256"); err != nil || !ok {
		t.Errorf("opcache.memory_consumption=256: ok=%v err=%v", ok, err)
	}
}
