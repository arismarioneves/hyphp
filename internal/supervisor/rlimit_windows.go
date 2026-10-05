package supervisor

// RaiseFileLimit não faz nada no Windows: não existe RLIMIT_NOFILE e os
// filhos não herdam um limite baixo de handles.
func RaiseFileLimit() error { return nil }
