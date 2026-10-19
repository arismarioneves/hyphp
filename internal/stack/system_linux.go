package stack

import (
	"context"

	"hyphp/internal/i18n"
	"hyphp/internal/netcfg"
)

// manageHosts: no Linux os domínios .test vêm do /etc/hosts, como no Windows.
func manageHosts() bool { return true }

// trustCA no Linux ainda não existe: a CA não entra na confiança do sistema.
func (s *Stack) trustCA(netcfg.Mkcert) error {
	return i18n.Errorf("err.linux.unavailable")
}

// SystemChanges: o "Remover do sistema" ainda não existe no Linux.
func (s *Stack) SystemChanges() SystemChanges { return SystemChanges{} }

// RemoveSystemChanges no Linux ainda não existe; a interface só oferece o
// botão quando SystemChanges().Supported.
func (s *Stack) RemoveSystemChanges(context.Context) error {
	return i18n.Errorf("err.linux.unavailable")
}
