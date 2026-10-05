package apache

import (
	"hyphp/internal/runtime"
	"hyphp/internal/webserver"
)

// httpd do Homebrew (keg em <prefix>/opt/httpd).
var plat = platform{
	// O formula do httpd instala os módulos em lib/httpd/modules, relativo ao
	// ServerRoot (= opt/httpd).
	ModulesDir: "lib/httpd/modules",
	// O Homebrew compila todos os MPMs como módulos e nenhum vem embutido: sem
	// um LoadModule de MPM o httpd -t falha. event é o adequado para
	// proxy_fcgi sem mod_php; unixd é exigido por qualquer MPM Unix.
	MPMModules: []string{"mpm_event", "unixd"},
	// --sysconfdir=<prefix>/etc/httpd põe o mime.types fora do keg.
	TypesConfig: func(inst runtime.Installed) string {
		return webserver.BrewPrefix(inst) + "/etc/httpd/mime.types"
	},
	// Sem letra de drive no Mac, a regex nunca casaria.
	DriveFix: false,
	// O padrão é <prefix>/var/run, compartilhado com o httpd do brew services.
	RuntimeDir: true,
	// No Unix o httpd se desliga do terminal: o supervisor veria exit 0,
	// reiniciaria e bateria em porta ocupada.
	ExtraArgs: []string{"-D", "FOREGROUND"},
}
