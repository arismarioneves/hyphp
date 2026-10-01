package render

import "testing"

// Letra de unidade só é caminho absoluto no Windows; no macOS
// "C:\Windows\..." é um nome de arquivo comum dentro de dir.
func TestWriteFilesRecusaCaminhoAbsolutoDoWindows(t *testing.T) {
	dir := t.TempDir()
	changed, err := WriteFiles(dir, map[string][]byte{`C:\Windows\System32\drivers\etc\hosts`: []byte("x")})
	if err == nil {
		t.Fatal("WriteFiles aceitou um caminho com letra de unidade")
	}
	if changed {
		t.Fatal("changed devia ser false quando a chave é recusada")
	}
}
