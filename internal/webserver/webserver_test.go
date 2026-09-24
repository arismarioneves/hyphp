package webserver

import "testing"

func TestPoolName(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"serie 8.1", "8.1", "php81"},
		{"serie 7.2", "7.2", "php72"},
		{"serie 8.4", "8.4", "php84"},
		{"patch completo nao e esperado mas nao quebra", "8.1.10", "php8110"},
		{"vazio", "", "php"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PoolName(tc.version); got != tc.want {
				t.Fatalf("PoolName(%q) = %q, quero %q", tc.version, got, tc.want)
			}
		})
	}
}
