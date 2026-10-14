package sysinfo

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// OS devolve o nome e a versão do Windows, como "Windows 11 (10.0.26100)".
// RtlGetVersion não passa pelo manifesto de compatibilidade, que faz o
// GetVersionEx mentir a versão.
func OS() string {
	v := windows.RtlGetVersion()
	return fmt.Sprintf("%s (%d.%d.%d)", windowsName(v.MajorVersion, v.BuildNumber), v.MajorVersion, v.MinorVersion, v.BuildNumber)
}

// windowsName separa o 10 do 11 pelo build: o Windows 11 continua se
// declarando 10.0 e começa no build 22000.
func windowsName(major, build uint32) string {
	switch {
	case major == 10 && build >= 22000:
		return "Windows 11"
	case major == 10:
		return "Windows 10"
	default:
		return "Windows"
	}
}
