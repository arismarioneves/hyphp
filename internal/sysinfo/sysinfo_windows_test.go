package sysinfo

import "testing"

func TestWindowsNameSeparaO10DoO11PeloBuild(t *testing.T) {
	casos := []struct {
		major, build uint32
		quer         string
	}{
		{10, 19045, "Windows 10"},
		{10, 21999, "Windows 10"},
		{10, 22000, "Windows 11"},
		{10, 26100, "Windows 11"},
		{6, 9600, "Windows"},
	}
	for _, c := range casos {
		if got := windowsName(c.major, c.build); got != c.quer {
			t.Errorf("windowsName(%d, %d) = %q; quero %q", c.major, c.build, got, c.quer)
		}
	}
}
