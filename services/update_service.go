package services

import (
	"context"
	"time"

	"hyphp/internal/update"
)

// checkTimeout cobre o manifesto e o download do instalador (~10 MB), que
// acontece dentro do mesmo Check quando há versão nova.
const checkTimeout = 30 * time.Minute

// UpdateService expõe o auto-update à UI. O estado chega também pelo evento
// update:status, emitido pelo próprio Updater a cada mudança.
type UpdateService struct {
	u    *update.Updater
	quit func()
}

func NewUpdateService(u *update.Updater, quit func()) *UpdateService {
	return &UpdateService{u: u, quit: quit}
}

// Status devolve o estado atual (a tela Sobre lê no primeiro render).
func (s *UpdateService) Status() update.Status { return s.u.Status() }

// Check verifica agora, independentemente do toggle automático.
func (s *UpdateService) Check() error {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	return s.u.Check(ctx)
}

// Apply fecha o app e instala a versão pronta. Se o atualizador não subir, o
// erro volta e o app continua aberto.
func (s *UpdateService) Apply() error { return s.u.Apply(s.quit) }
