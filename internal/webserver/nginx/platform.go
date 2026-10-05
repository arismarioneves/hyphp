package nginx

import "hyphp/internal/runtime"

// platform reúne o que muda entre o nginx do Windows e o do Homebrew. Os
// templates são os mesmos; só estes dados variam (valores em platform_<so>.go).
type platform struct {
	// ConfDir é onde estão o mime.types e o fastcgi_params da instalação.
	ConfDir func(inst runtime.Installed) string
	// TempPaths fixa os *_temp_path em temp/ dentro do prefixo -p.
	TempPaths bool
	// RlimitNofile, se > 0, vira worker_rlimit_nofile.
	RlimitNofile int
	// ExtraArgs vão no Validate e no Command.
	ExtraArgs []string
}
