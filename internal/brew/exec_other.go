//go:build !darwin

package brew

import (
	"context"
	"errors"
)

// Locate fora do macOS: os runtimes do Windows vêm do catálogo, não do Homebrew.
func Locate(ctx context.Context) (Brew, error) {
	return Brew{}, errors.ErrUnsupported
}
