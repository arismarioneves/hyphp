//go:build !windows

package main

// callerName fora do Windows fica vazio: a aba CLI mostra a chamada sem o
// programa que a fez.
func callerName() string { return "" }
