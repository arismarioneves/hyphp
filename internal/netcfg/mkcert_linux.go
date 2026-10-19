package netcfg

// CAInstalled no Linux é sempre falso: o HyPHP ainda não instala a CA na
// confiança do sistema, e sem isso os sites .test não abrem por HTTPS sem
// aviso, mesmo com a CA criada em disco.
func (m Mkcert) CAInstalled() (bool, error) { return false, nil }
