package main

import (
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// shells são intermediários: dizer só "bash.exe" esconderia quem abriu o
// shell (um agente de IA, um editor). Com um deles na frente, a atividade
// mostra também o próximo processo acima.
var shells = map[string]bool{
	"cmd.exe": true, "powershell.exe": true, "pwsh.exe": true, "bash.exe": true,
	"sh.exe": true, "zsh.exe": true, "wsl.exe": true, "conhost.exe": true, "openconsole.exe": true,
}

// callerName devolve o processo que chamou a CLI ("pwsh.exe", ou
// "bash.exe ← claude.exe" quando o pai é um shell). Vazio se não der para ler.
func callerName() string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(snap)
	type proc struct {
		parent uint32
		exe    string
	}
	procs := map[uint32]proc{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		procs[e.ProcessID] = proc{parent: e.ParentProcessID, exe: windows.UTF16ToString(e.ExeFile[:])}
	}
	me, ok := procs[uint32(os.Getpid())]
	if !ok {
		return ""
	}
	parent, ok := procs[me.parent]
	if !ok {
		return ""
	}
	name := parent.exe
	// Sobe enquanto for shell, no máximo 4 níveis: o PID do pai pode já ter
	// sido reusado, e um ciclo não pode travar a CLI.
	cur := parent
	for range 4 {
		if !shells[strings.ToLower(cur.exe)] {
			break
		}
		up, ok := procs[cur.parent]
		if !ok || up.exe == "" {
			break
		}
		if !shells[strings.ToLower(up.exe)] {
			return name + " ← " + up.exe
		}
		cur = up
	}
	return name
}
