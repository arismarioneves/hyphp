package stack

import "hyphp/internal/runtime"

// syncCACert no Mac não faz nada: o PHP do Homebrew usa os certificados de CA
// do próprio Homebrew.
func (s *Stack) syncCACert([]runtime.Installed) (string, []Warning) { return "", nil }
