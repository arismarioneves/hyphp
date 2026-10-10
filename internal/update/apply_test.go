package update

import (
	"strings"
	"testing"
)

// O atualizador recebe tudo por linha de comando; um campo que não faça a ida
// e volta chega vazio e a reconferência do pacote deixa de existir.
func TestApplyArgsIdaEVolta(t *testing.T) {
	req := ApplyRequest{
		PID: 1234, Installer: `C:\HyPHP\var\update\3.1.0\setup.exe`, SHA256: strings.Repeat("a", 64), Size: 4096,
		Dir: `C:\Program Files\HyPHP`, Exe: "hyphp.exe", Result: `C:\HyPHP\var\update\resultado.json`, From: "3.0.0", To: "3.1.0",
	}
	got, err := parseApplyArgs(req.args())
	if err != nil {
		t.Fatal(err)
	}
	if got != req {
		t.Fatalf("ida e volta:\n got %+v\nwant %+v", got, req)
	}
}

func TestApplyArgsSemHashRecusa(t *testing.T) {
	req := ApplyRequest{Installer: "x.exe", Dir: "d", Exe: "hyphp.exe", Result: "r.json", Size: 10}
	if _, err := parseApplyArgs(req.args()); err == nil {
		t.Fatal("sem sha256 o atualizador não pode seguir")
	}
}
