package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// desktopName é fixo para que ligar duas vezes sobrescreva o mesmo arquivo e
// desligar encontre o que gravamos.
const desktopName = "com.hyphp.app.desktop"

// autostartDir permite aos testes usar um diretório temporário em vez do
// ~/.config/autostart real, que apagaria o autostart do HyPHP instalado.
var autostartDir = ""

// targetDir é $XDG_CONFIG_HOME/autostart, com ~/.config como padrão da
// especificação XDG. A sessão gráfica abre o que estiver ali no login.
func targetDir() (string, error) {
	if autostartDir != "" {
		return autostartDir, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(d) {
		return filepath.Join(d, "autostart"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("autostart: localizar diretório home: %w", err)
	}
	return filepath.Join(home, ".config", "autostart"), nil
}

// execLine monta a chave Exec do .desktop. O executável vai entre aspas, com
// as regras da especificação Desktop Entry: dentro das aspas, ", `, $ e \
// ganham uma barra; depois cada barra é dobrada, porque o valor da chave
// também é uma string com escapes; e % vira %%, que não é código de campo.
// Enabled compara com a mesma linha.
func execLine(exe string) string {
	var b strings.Builder
	for _, r := range exe {
		if strings.ContainsRune("\"`$\\", r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	quoted := strings.ReplaceAll(b.String(), `\`, `\\`)
	quoted = strings.ReplaceAll(quoted, "%", "%%")
	return `Exec="` + quoted + `" ` + Flag
}

// Apply cria ou remove o .desktop de autostart.
func Apply(enabled bool) error {
	dir, err := targetDir()
	if err != nil {
		return err
	}
	target := filepath.Join(dir, desktopName)

	if !enabled {
		// Remover o que não existe é sucesso: Set() das configurações chama
		// Apply a cada gravação, mesmo quando o toggle não mudou.
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("autostart: remover %s: %w", target, err)
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
	content := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=HyPHP\n" +
		execLine(exe) + "\n" +
		"Icon=hyphp\n" +
		"Terminal=false\n" +
		"X-GNOME-Autostart-enabled=true\n"
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return fmt.Errorf("autostart: gravar %s: %w", target, err)
	}
	return nil
}

// Enabled responde se o .desktop existe e abre o executável atual: um arquivo
// deixado por outra instalação não conta como ligado.
func Enabled() (bool, error) {
	dir, err := targetDir()
	if err != nil {
		return false, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, desktopName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("autostart: ler .desktop: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("autostart: localizar executável: %w", err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if line == execLine(exe) {
			return true, nil
		}
	}
	return false, nil
}
