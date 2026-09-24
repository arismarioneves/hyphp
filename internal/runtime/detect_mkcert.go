package runtime

import "context"

func detectMkcert(ctx context.Context, dir, exe string) (Installed, error) {
	out, err := run(ctx, dir, exe, "-version")
	if err != nil {
		return Installed{}, err
	}
	version, err := parseFirstVersion(out, "mkcert")
	if err != nil {
		return Installed{}, err
	}
	return Installed{Kind: Mkcert, Version: version, Major: version, Dir: dir, Exe: exe}, nil
}
