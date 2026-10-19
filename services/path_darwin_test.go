package services

import (
	"os"
	"path/filepath"
	"testing"

	"hyphp/internal/paths"
)

// A aba CLI diz "no PATH" pelo /etc/paths.d, não pelo PATH do app (de GUI,
// sem o PATH do shell).
func TestNoPathPeloPathsD(t *testing.T) {
	raizTemp(t)
	paths.PathsDFile = filepath.Join(t.TempDir(), "hyphp")
	t.Cleanup(func() { paths.PathsDFile = "/etc/paths.d/hyphp" })
	onPath := func() bool {
		p, err := readUserPath()
		if err != nil {
			t.Fatal(err)
		}
		_, mudou := pathComDir(p, cliPathDir("/x/hyphp"))
		return !mudou
	}
	if onPath() {
		t.Fatal("no PATH sem paths.d")
	}
	if err := os.WriteFile(paths.PathsDFile, []byte(paths.PathsDContent(paths.Cli())), 0o644); err != nil {
		t.Fatal(err)
	}
	if !onPath() {
		t.Fatal("fora do PATH com o paths.d registrado")
	}
}
