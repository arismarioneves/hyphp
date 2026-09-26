//go:build production

package update

// BuildEnabled liga o auto-update. A tag production é a que o Taskfile passa
// no build do instalador; o bin/hyphp-dbg.exe não a tem.
const BuildEnabled = true
