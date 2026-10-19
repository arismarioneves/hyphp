//go:build windows || linux

package stack

import (
	"sort"

	"hyphp/internal/project"
)

// wildcardDomains devolve os sufixos dos projetos com wildcard ligado: onde os
// domínios .test vêm do hosts, só eles dependem do resolvedor.
func wildcardDomains(projs []project.Project) []string {
	var out []string
	for _, p := range projs {
		if p.Wildcard {
			out = append(out, p.Domain)
		}
	}
	sort.Strings(out)
	return out
}
