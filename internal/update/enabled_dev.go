//go:build !production

package update

// BuildEnabled fica desligado fora de produção: um build de dev que se
// atualizasse instalaria a versão publicada por cima de bin/.
const BuildEnabled = false
