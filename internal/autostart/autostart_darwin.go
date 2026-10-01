// Package autostart liga e desliga a abertura do HyPHP no login do usuário.
//
// No macOS isso é um LaunchAgent em ~/Library/LaunchAgents. O launchd lê os
// plists dessa pasta no próximo login, então basta gravar ou remover o arquivo:
// não é preciso chamar `launchctl` (o que, aliás, abriria uma segunda instância
// agora, já que RunAtLoad dispara no carregamento).
package autostart

import (
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

// plistName é fixo para que ligar duas vezes sobrescreva o mesmo arquivo e
// desligar encontre o que gravamos.
const plistName = "com.hyphp.app.plist"

// launchAgentsDir permite aos testes usar um diretório temporário em vez do
// ~/Library/LaunchAgents real, que apagaria o autostart do HyPHP instalado.
var launchAgentsDir = ""

func targetDir() (string, error) {
	if launchAgentsDir != "" {
		return launchAgentsDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("autostart: localizar diretório home: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents"), nil
}

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.hyphp.app</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`

// exeString devolve o elemento XML do executável atual. O caminho é escapado
// porque um `&` ou `<` nele tornaria o plist inválido; Enabled usa a mesma
// função para comparar exatamente o que Apply grava.
func exeString() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("autostart: localizar executável: %w", err)
	}
	return "<string>" + html.EscapeString(exe) + "</string>", nil
}

// Apply cria ou remove o plist do LaunchAgent.
func Apply(enabled bool) error {
	dir, err := targetDir()
	if err != nil {
		return err
	}
	target := filepath.Join(dir, plistName)

	if !enabled {
		// Remover o que não existe é sucesso: Set() das configurações chama
		// Apply a cada gravação, mesmo quando o toggle não mudou.
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("autostart: remover plist: %w", err)
		}
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("autostart: localizar executável: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("autostart: criar diretório %s: %w", dir, err)
	}
	content := fmt.Sprintf(plistTemplate, html.EscapeString(exe))
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return fmt.Errorf("autostart: gravar plist: %w", err)
	}
	return nil
}

// Enabled responde se o plist existe e aponta para o executável atual: um
// plist deixado por outra instalação não conta como ligado.
func Enabled() (bool, error) {
	dir, err := targetDir()
	if err != nil {
		return false, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, plistName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("autostart: ler plist: %w", err)
	}
	want, err := exeString()
	if err != nil {
		return false, err
	}
	return strings.Contains(string(raw), want), nil
}
