package sysinfo

import "golang.org/x/sys/unix"

// OS devolve "macOS" e a versão do produto, como "macOS 15.1".
func OS() string {
	v, err := unix.Sysctl("kern.osproductversion")
	if err != nil || v == "" {
		return "macOS"
	}
	return "macOS " + v
}
