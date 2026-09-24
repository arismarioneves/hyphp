package runtime

import "context"

func detectMailpit(ctx context.Context, dir, exe string) (Installed, error) {
	out, err := run(ctx, dir, exe, "version")
	if err != nil {
		return Installed{}, err
	}
	version, err := parseFirstVersion(out, "mailpit")
	if err != nil {
		return Installed{}, err
	}
	return Installed{Kind: Mailpit, Version: version, Major: version, Dir: dir, Exe: exe}, nil
}
