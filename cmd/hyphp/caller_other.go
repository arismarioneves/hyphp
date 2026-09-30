//go:build !windows

package main

// callerName fora do Windows fica vazio: o app só é empacotado para Windows.
func callerName() string { return "" }
