package brew

import (
	"context"
	"errors"
)

// Locate no Windows: os runtimes vêm do catálogo, não do Homebrew.
func Locate(ctx context.Context) (Brew, error) {
	return Brew{}, errors.ErrUnsupported
}
