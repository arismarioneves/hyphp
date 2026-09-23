package services

import (
	"log/slog"

	"hyphp/internal/supervisor"
)

// ServicesDeps são as dependências injetadas por main.go.
type ServicesDeps struct {
	Sup    *supervisor.Supervisor
	Logger *slog.Logger
}

// ServicesService expõe a máquina de estados do supervisor à tela Serviços.
type ServicesService struct {
	d ServicesDeps
}

func NewServicesService(d ServicesDeps) *ServicesService {
	return &ServicesService{d: d}
}

// List devolve todos os serviços ordenados por grupo e ID.
func (s *ServicesService) List() []supervisor.Status {
	return s.d.Sup.List()
}

// Start inicia um serviço. Devolve assim que o processo nasce; Ready ou
// Failed chegam pelo evento service:state.
func (s *ServicesService) Start(id string) error {
	return s.d.Sup.Start(id)
}

// Stop bloqueia até o serviço estar parado.
func (s *ServicesService) Stop(id string) error {
	return s.d.Sup.Stop(id)
}

func (s *ServicesService) Restart(id string) error {
	return s.d.Sup.Restart(id)
}

func (s *ServicesService) StartAll() error {
	return s.d.Sup.StartAll()
}

func (s *ServicesService) StopAll() error {
	return s.d.Sup.StopAll()
}
