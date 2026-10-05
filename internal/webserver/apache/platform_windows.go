package apache

import (
	hrender "hyphp/internal/render"
	"hyphp/internal/runtime"
)

// Build Apache Lounge: tudo dentro do diretório da instalação, mpm_winnt
// embutido e o httpd.exe já fica em primeiro plano.
var plat = platform{
	ModulesDir: "modules",
	TypesConfig: func(inst runtime.Installed) string {
		return hrender.SlashDir(inst.Dir) + "/conf/mime.types"
	},
	DriveFix: true,
}
