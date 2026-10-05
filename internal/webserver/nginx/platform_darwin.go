package nginx

import (
	"hyphp/internal/runtime"
	"hyphp/internal/webserver"
)

// nginx do Homebrew: os caminhos compilados são absolutos no prefixo do brew
// e compartilhados com o nginx do brew services.
var plat = platform{
	// --conf-path=<prefix>/etc/nginx/nginx.conf põe mime.types e
	// fastcgi_params fora do keg.
	ConfDir: func(inst runtime.Installed) string {
		return webserver.BrewPrefix(inst) + "/etc/nginx"
	},
	// Os temp paths compilados ficam em <prefix>/var/run/nginx; trazê-los para
	// temp/ do prefixo -p usa o temp/.keep que o Render já cria.
	TempPaths: true,
	// O limite padrão de arquivos do macOS (256) é baixo para worker_connections
	// 1024 com upstreams FastCGI.
	RlimitNofile: 4096,
	// O log de erro compilado (<prefix>/var/log/nginx) é aberto antes da
	// config; -e o traz para logs/ do prefixo, que sempre existe.
	ExtraArgs: []string{"-e", "logs/error.log"},
}
