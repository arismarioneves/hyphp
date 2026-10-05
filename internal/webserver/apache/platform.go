package apache

import "hyphp/internal/runtime"

// platform reúne o que muda no httpd.conf e na linha de comando entre o build
// Apache Lounge do Windows e o httpd do Homebrew. Os templates são os mesmos;
// só estes dados variam (valores em platform_<so>.go).
type platform struct {
	// ModulesDir é relativo ao ServerRoot.
	ModulesDir string
	// MPMModules são carregados antes da lista comum: o MPM e o unixd são
	// pré-requisito dos demais no Unix. No Windows o mpm_winnt é embutido.
	MPMModules []string
	// TypesConfig devolve o caminho do mime.types da instalação.
	TypesConfig func(inst runtime.Installed) string
	// DriveFix emite o ProxyFCGISetEnvIf da letra de drive (spec §6.2).
	DriveFix bool
	// RuntimeDir fixa DefaultRuntimeDir e Mutex na pasta de log do HyPHP.
	RuntimeDir bool
	// ExtraArgs vão só no Command, nunca no -t.
	ExtraArgs []string
}
