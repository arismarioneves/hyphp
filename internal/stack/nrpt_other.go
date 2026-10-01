//go:build !windows

package stack

// nrptRuleExists: NRPT é só do Windows; fora dele não há regra a encontrar.
func nrptRuleExists(string) bool { return false }
