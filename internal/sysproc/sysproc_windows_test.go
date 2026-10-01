package sysproc

import (
	"os/exec"
	"syscall"
	"testing"
)

// O editor monta a linha do cmd.exe à mão em SysProcAttr.CmdLine; esconder o
// console não pode apagar essa linha, senão o VS Code abre a pasta errada.
func TestHidePreservaCmdLine(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /c code "C:\a&b"`, CreationFlags: 0x200}
	Hide(cmd)
	a := cmd.SysProcAttr
	if a.CmdLine != `cmd.exe /c code "C:\a&b"` || !a.HideWindow || a.CreationFlags != 0x200|createNoWindow {
		t.Fatalf("SysProcAttr = %+v", a)
	}
}

func TestExeNameAcrescentaExe(t *testing.T) {
	for in, want := range map[string]string{"mysql": "mysql.exe", "mkcert.exe": "mkcert.exe", "PHP.EXE": "PHP.EXE"} {
		if got := ExeName(in); got != want {
			t.Errorf("ExeName(%q) = %q, want %q", in, got, want)
		}
	}
}
