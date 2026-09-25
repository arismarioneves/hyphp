package runtime

import "context"

// detectMailpit lê a versão de `mailpit version`.
//
// O exit code é ignorado de propósito: o mailpit 1.31.1 imprime
// "mailpit v1.31.1 compiled with go1.27.1 on windows/amd64" e ainda assim
// termina com status 1. Descartar a saída por causa disso deixava o Mailpit
// invisível — nenhum serviço de e-mail subia, com o binário instalado e
// funcionando. O que decide é haver versão na saída, não o código de saída.
func detectMailpit(ctx context.Context, dir, exe string) (Installed, error) {
	out, _ := run(ctx, dir, exe, "version")
	version, err := parseFirstVersion(out, "mailpit")
	if err != nil {
		return Installed{}, err
	}
	return Installed{Kind: Mailpit, Version: version, Major: version, Dir: dir, Exe: exe}, nil
}
