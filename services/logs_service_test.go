package services

import (
	"strings"
	"testing"
)

func TestParseHello(t *testing.T) {
	tests := []struct {
		name    string
		frame   string
		wantID  string
		wantErr string // "" = sucesso
	}{
		{"hello válido", `{"id":"php:8.1:0"}`, "php:8.1:0", ""},
		{"campo extra é ignorado", `{"id":"mysql","tail":100}`, "mysql", ""},
		{"id vazio", `{"id":""}`, "", "sem campo id"},
		{"sem campo id", `{}`, "", "sem campo id"},
		{"json inválido", `{id: mysql}`, "", "hello inválido"},
		{"frame vazio", ``, "", "hello inválido"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id, err := parseHello([]byte(tc.frame))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("parseHello() err = %v, want nil", err)
				}
				if id != tc.wantID {
					t.Fatalf("parseHello() id = %q, want %q", id, tc.wantID)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("parseHello() err = %v, want contendo %q", err, tc.wantErr)
			}
		})
	}
}
