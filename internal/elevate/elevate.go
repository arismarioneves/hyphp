package elevate

import (
	"errors"
	"fmt"
)

var (
	// ErrElevationDenied: o usuário recusou o UAC. Como só ações explícitas
	// elevam (Stack.ApplyHosts, Stack.InstallCA), isso vira erro da ação e
	// aparece na própria tela — não um warning do Reconcile.
	ErrElevationDenied = errors.New("elevação negada pelo usuário (UAC)")
	// ErrHelperFailed: o helper rodou e terminou com código ≠ 0. Sentinela de HelperError.
	ErrHelperFailed = errors.New("hyphp-helper falhou")
	// ErrHelperTimeout: o helper não terminou em helperTimeoutMS.
	ErrHelperTimeout = errors.New("hyphp-helper não terminou a tempo")
)

// HelperError descreve uma execução do helper que terminou com código ≠ 0.
// Message vem do JSON que o helper grava no arquivo passado em --result.
type HelperError struct {
	ExitCode int
	Message  string
}

func (e *HelperError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("hyphp-helper terminou com código %d", e.ExitCode)
	}
	return fmt.Sprintf("hyphp-helper terminou com código %d: %s", e.ExitCode, e.Message)
}

// Unwrap liga HelperError à sentinela: errors.Is(err, ErrHelperFailed) funciona,
// e quem precisa do código usa errors.As(err, &he).
func (e *HelperError) Unwrap() error { return ErrHelperFailed }
