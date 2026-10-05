package supervisor

import (
	"log/slog"
	"os/exec"
)

// No Windows o kernel já garante que não sobra órfão: o hyphp.exe e todo
// serviço nascem dentro do Job Object global kill-on-close (attachSelf), e
// quando o app morre, por qualquer motivo, o kernel fecha o último handle do
// job e mata a árvore inteira. Não há registro a gravar nem órfão a procurar.

func recordProcess(string, Spec, *exec.Cmd, procHandle) error { return nil }

func forgetProcess(string, string) {}

// ReapOrphans não tem o que encerrar no Windows (ver acima).
func ReapOrphans(string, *slog.Logger) []string { return nil }
