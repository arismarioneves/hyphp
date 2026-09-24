package runtime

import (
	"context"
	"errors"
)

func detectPHP(ctx context.Context, dir, exe string) (Installed, error) { return Installed{}, errStub }
func detectApache(ctx context.Context, dir, exe string) (Installed, error) {
	return Installed{}, errStub
}
func detectNginx(ctx context.Context, dir, exe string) (Installed, error) {
	return Installed{}, errStub
}
func detectMySQL(ctx context.Context, dir, exe string) (Installed, error) {
	return Installed{}, errStub
}
func detectMailpit(ctx context.Context, dir, exe string) (Installed, error) {
	return Installed{}, errStub
}
func detectMkcert(ctx context.Context, dir, exe string) (Installed, error) {
	return Installed{}, errStub
}

var errStub = errors.New("runtime: detector não implementado (Task 2)")
