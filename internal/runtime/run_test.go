package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestHelperProcess não é teste: é o processo filho de TestRunDevolveSaidaComExitNaoZero,
// imitando o mailpit que imprime a versão e sai com status 1.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("HYPHP_TEST_HELPER") != "1" {
		return
	}
	fmt.Println("mailpit v1.21.8 compiled with go1.23")
	os.Exit(1)
}

func TestRunDevolveSaidaComExitNaoZero(t *testing.T) {
	t.Setenv("HYPHP_TEST_HELPER", "1")
	out, err := run(context.Background(), t.TempDir(), os.Args[0], "-test.run=^TestHelperProcess$")
	if err == nil {
		t.Fatal("esperava erro pelo exit 1")
	}
	if !strings.Contains(out, "v1.21.8") {
		t.Fatalf("saída perdida no caminho de erro: %q", out)
	}
	v, err := parseFirstVersion(out, "mailpit")
	if err != nil || v != "1.21.8" {
		t.Fatalf("parseFirstVersion = %q, %v", v, err)
	}
}
