package project

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func observando(w *Watcher, dir string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.watched[dir]
}

// Pasta de projeto apagada e recriada (rmdir + git clone) tem de voltar a ser
// observada no SetRoots seguinte. Antes, watched continuava true e o Add
// nunca era repetido.
func TestWatcherEsqueceDiretorioRemovido(t *testing.T) {
	root, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := mkdirs(t, root, "proj")
	w, err := NewWatcher(func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.SetRoots([]string{root}); err != nil {
		t.Fatal(err)
	}
	if !observando(w, proj) {
		t.Fatal("proj devia estar observado após SetRoots")
	}

	if err := os.RemoveAll(proj); err != nil {
		t.Fatal(err)
	}
	limite := time.Now().Add(5 * time.Second)
	for observando(w, proj) {
		if time.Now().After(limite) {
			t.Fatal("proj removido continuou marcado como observado")
		}
		time.Sleep(10 * time.Millisecond)
	}

	mkdirs(t, root, "proj")
	if err := w.SetRoots([]string{root}); err != nil {
		t.Fatal(err)
	}
	if !observando(w, proj) {
		t.Fatal("proj recriado devia voltar a ser observado")
	}
}
