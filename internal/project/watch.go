package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// debounce agrupa rajadas de eventos (git checkout, npm install) num só onChange.
const debounce = 500 * time.Millisecond

// Watcher observa cada root (para projetos aparecendo/sumindo) e cada
// <root>/<filho>/ (para hyphp.yaml, public/, .htaccess mudando). Não é
// recursivo: fsnotify no Windows observa um diretório por watch e um projeto
// Laravel tem dezenas de milhares de arquivos. Cada evento reinicia um timer de
// 500ms; ao expirar, onChange é chamado uma vez, fora de qualquer lock.
type Watcher struct {
	fs       *fsnotify.Watcher
	onChange func()

	mu      sync.Mutex
	watched map[string]bool
	timer   *time.Timer
	closed  bool
}

// NewWatcher cria o watcher e inicia a goroutine de eventos. Nenhum diretório
// é observado até SetRoots.
func NewWatcher(onChange func()) (*Watcher, error) {
	fs, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("project: criar watcher: %w", err)
	}
	w := &Watcher{fs: fs, onChange: onChange, watched: map[string]bool{}}
	go w.loop()
	return w, nil
}

// SetRoots substitui o conjunto observado por roots + filhos diretos (dirs não
// ocultos, exceto node_modules/vendor). Idempotente: chamar após cada rescan
// garante que projetos novos passam a ser observados. Roots inexistentes são
// ignorados sem erro.
func (w *Watcher) SetRoots(roots []string) error {
	want := map[string]bool{}
	for _, root := range roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if st, err := os.Stat(abs); err != nil || !st.IsDir() {
			continue
		}
		want[abs] = true
		entries, err := os.ReadDir(abs)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Name()[0] == '.' || skipDirs[e.Name()] || !isDirEntry(abs, e) {
				continue
			}
			want[filepath.Join(abs, e.Name())] = true
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("project: watcher fechado")
	}
	var firstErr error
	for dir := range w.watched {
		if !want[dir] {
			_ = w.fs.Remove(dir) // dir pode já não existir; ignorar
			delete(w.watched, dir)
		}
	}
	// ordem estável para mensagens de erro determinísticas
	dirs := make([]string, 0, len(want))
	for dir := range want {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		if w.watched[dir] {
			continue
		}
		if err := w.fs.Add(dir); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("project: observar %s: %w", dir, err)
			}
			continue
		}
		w.watched[dir] = true
	}
	return firstErr
}

// Close para a goroutine e libera os handles do sistema.
func (w *Watcher) Close() error {
	w.mu.Lock()
	w.closed = true
	if w.timer != nil {
		w.timer.Stop()
	}
	w.mu.Unlock()
	return w.fs.Close()
}

func (w *Watcher) loop() {
	for {
		select {
		case ev, ok := <-w.fs.Events:
			if !ok {
				return
			}
			// só interessa o que muda a descoberta: criar/remover/renomear
			// diretórios e escrever hyphp.yaml/.htaccess/index.php/composer.json.
			if ev.Has(fsnotify.Chmod) {
				continue
			}
			// O fsnotify descarta o watch de um diretório removido, mas
			// watched continuaria true e SetRoots nunca o observaria de novo
			// quando a pasta fosse recriada (rmdir + git clone).
			if ev.Has(fsnotify.Remove) {
				w.forget(ev.Name)
			}
			w.bump()
		case _, ok := <-w.fs.Errors:
			if !ok {
				return
			}
			// erro de overflow do buffer do Windows: melhor um rescan a mais que a menos
			w.bump()
		}
	}
}

// forget só esquece o diretório: chamar w.fs.Remove daqui, a goroutine que
// consome Events, pode travar com o fsnotify bloqueado entregando o próximo
// evento — e o watch de um diretório removido já foi descartado por ele.
func (w *Watcher) forget(dir string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.watched, dir)
}

func (w *Watcher) bump() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	if w.timer == nil {
		w.timer = time.AfterFunc(debounce, w.fire)
		return
	}
	w.timer.Reset(debounce)
}

func (w *Watcher) fire() {
	w.mu.Lock()
	closed := w.closed
	w.mu.Unlock()
	if !closed {
		w.onChange()
	}
}
