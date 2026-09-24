// Package autostart liga e desliga a abertura do HyPHP no login do usuário.
package autostart

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

// runKey é a chave Run do USUÁRIO. A equivalente em HKLM valeria para a
// máquina inteira e exigiria UAC — o HyPHP não eleva para abrir (spec §11).
const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// valueName é o nome da entrada; fixo para que ligar duas vezes não crie
// duplicata e para que desligar encontre o que gravamos.
const valueName = "HyPHP"

// Apply cria ou remove a entrada de autostart.
func Apply(enabled bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("autostart: abrir %s: %w", runKey, err)
	}
	defer k.Close()

	if !enabled {
		// Remover o que não existe é sucesso: Set() das configurações chama
		// Apply a cada gravação, mesmo quando o toggle não mudou.
		if err := k.DeleteValue(valueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("autostart: remover entrada: %w", err)
		}
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("autostart: localizar executável: %w", err)
	}
	// As aspas são obrigatórias: o caminho de instalação tem espaço
	// ("C:\Program Files\HyPHP\hyphp.exe") e sem elas o Windows tentaria
	// executar "C:\Program".
	if err := k.SetStringValue(valueName, `"`+exe+`"`); err != nil {
		return fmt.Errorf("autostart: gravar entrada: %w", err)
	}
	return nil
}

// Enabled responde se a entrada existe.
func Enabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("autostart: abrir %s: %w", runKey, err)
	}
	defer k.Close()

	switch _, _, err := k.GetStringValue(valueName); {
	case errors.Is(err, registry.ErrNotExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("autostart: ler entrada: %w", err)
	}
	return true, nil
}
