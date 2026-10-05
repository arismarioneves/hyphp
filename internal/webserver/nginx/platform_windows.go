package nginx

import (
	hrender "hyphp/internal/render"
	"hyphp/internal/runtime"
)

// nginx para Windows: conf/ dentro da instalação, e os caminhos compilados de
// log e temp já são relativos ao prefixo (daí os .keep de logs/ e temp/).
var plat = platform{
	ConfDir: func(inst runtime.Installed) string {
		return hrender.SlashDir(inst.Dir) + "/conf"
	},
}
