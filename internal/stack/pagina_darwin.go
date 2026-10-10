package stack

// resolucao no Mac: sem /etc/hosts, todo .test depende da regra
// /etc/resolver/test e do resolvedor no ar.
func (s *Stack) resolucao() ([]string, bool, string) {
	return nil, s.resolver != nil && resolverRuleState() == ruleOurs, "dns"
}
