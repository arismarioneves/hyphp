package stack

import (
	"context"
	"errors"
	"path/filepath"

	"hyphp/internal/elevate"
	"hyphp/internal/i18n"
	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
	"hyphp/internal/state"
)

// manageHosts: no Windows o hosts é o caminho dos domínios .test (a regra
// NRPT só cobre os subdomínios de projetos com wildcard).
func manageHosts() bool { return true }

// mkcertMissingMessage: no Windows o mkcert é baixado para bin/mkcert pela
// tela Runtimes.
func mkcertMissingMessage() string {
	return i18n.T("warn.mkcertMissing", filepath.Join(paths.Bin(), "mkcert"))
}

// mkcertMissingError: no Windows o texto cita bin/mkcert/mkcert.exe.
func mkcertMissingError() error { return i18n.Errorf("err.stack.mkcertMissing") }

// webMissingMessage: no Windows o web server é baixado ou importado para
// bin/<nome>. O caminho completo é obrigatório na mensagem: "não está
// instalado em bin/" leva o usuário a olhar o bin/ errado quando existe outra
// ferramenta na máquina com um Apache próprio.
func webMissingMessage(name state.WebServerName) string {
	return i18n.T("warn.webMissing", name, filepath.Join(paths.Bin(), string(name)))
}

// hostsCancelled: no Windows a gravação do hosts sobe pelo UAC.
func hostsCancelled(change string) error { return i18n.Errorf("err.stack.hostsCancelled", change) }

// portsBelowLimit: o Windows abre as portas baixas sem administrador.
func portsBelowLimit(state.State) []Warning { return nil }

// ApplyPorts: no Windows as portas baixas abrem sem UAC, então não há limite
// a liberar.
func (s *Stack) ApplyPorts(context.Context) error { return nil }

// syncBrowserTrust: no Windows a CA vai para os navegadores só no InstallCA.
func (s *Stack) syncBrowserTrust(netcfg.Mkcert) {}

// trustCA instala a CA pelo helper elevado (mkcert -install sob UAC).
func (s *Stack) trustCA(mk netcfg.Mkcert) error {
	helper, herr := elevate.HelperPath()
	if herr != nil {
		return i18n.Errorf("err.stack.helperUnavailable", herr)
	}
	// --caroot fixa no helper elevado o CAROOT deste usuário: se o UAC elevar
	// com outra conta, o mkcert -install gravaria a CA no perfil dela, e
	// CAInstalled(), que olha aqui, seguiria falso a cada clique.
	switch err := elevate.RunElevated(helper, []string{"mkcert-install", "--exe", mk.Exe, "--caroot", mk.CARoot}); {
	case errors.Is(err, elevate.ErrElevationDenied):
		return i18n.Errorf("err.stack.caCancelled")
	case err != nil:
		return i18n.Errorf("err.stack.caInstallFailed", err)
	}
	return nil
}

// SystemChanges: o "Remover do sistema" é só do Mac.
func (s *Stack) SystemChanges() SystemChanges { return SystemChanges{} }

// RemoveSystemChanges não existe no Windows: a interface só oferece o botão
// quando SystemChanges().Supported.
func (s *Stack) RemoveSystemChanges(context.Context) error {
	return errors.New(i18n.T("err.stack.removeUnsupported"))
}
