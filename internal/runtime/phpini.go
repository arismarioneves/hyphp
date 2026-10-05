package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// iniNameRe é o formato de nome aceito para diretivas definidas pelo usuário.
// É mais estreito que o do PHP de propósito: o nome vai embutido entre aspas
// simples no script de -r e num argumento -d, então nada de aspas, espaço, `;`
// ou `=`. Todas as diretivas do PHP e das extensões oficiais cabem nele.
var iniNameRe = regexp.MustCompile(`^[a-z][a-z0-9_.]*$`)

// ValidIniName diz se name pode ser consultado/escrito com segurança.
func ValidIniName(name string) bool {
	return iniNameRe.MatchString(name)
}

// iniMarker separa a resposta do script de qualquer warning que o PHP imprima
// antes (run() combina stdout e stderr, e uma DLL de extensão quebrada fala em
// stdout na CLI).
const iniMarker = "@@HYPHP-INI@@"

// cliHardcoded são os valores que a SAPI CLI impõe por cima dos builtins
// (HARDCODED_INI em sapi/cli/php_cli.c), com o builtin real do PHP ao lado.
// Os workers são php-cgi, que não impõe nada: sem esta correção a UI diria
// que o padrão do PHP para max_execution_time é 0 (sem limite), o que é falso
// para uma página.
var cliHardcoded = map[string]string{
	"html_errors":        "1",
	"implicit_flush":     "0",
	"max_execution_time": "30",
}

// PHPIniBuiltins devolve o valor builtin de cada diretiva em names — o que o
// PHP usa sem php.ini algum (-n). Carrega as extensões de ext para que as
// diretivas delas (opcache.*, mysqli.*) existam. Diretiva desconhecida fica
// fora do mapa.
func PHPIniBuiltins(ctx context.Context, inst Installed, ext []string, names []string) (map[string]string, error) {
	out := map[string]string{}
	if len(names) == 0 {
		return out, nil
	}
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		if !ValidIniName(n) {
			return nil, fmt.Errorf("runtime: nome de diretiva inválido %q", n)
		}
		quoted = append(quoted, fmt.Sprintf("'%s'=>ini_get('%s')", n, n))
	}
	script := "echo PHP_EOL.'" + iniMarker + "'.json_encode([" + strings.Join(quoted, ",") + "]);"
	args := append(extensionArgs(inst, ext), "-r", script)
	raw, err := runIniScript(ctx, inst, args)
	if err != nil {
		return nil, err
	}
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("runtime: saída inesperada do php: %q", raw)
	}
	for name, v := range values {
		if s, ok := v.(string); ok {
			out[name] = s
			if fixed, ok := cliHardcoded[name]; ok {
				out[name] = fixed
			}
		}
	}
	return out, nil
}

// PHPIniKnows pergunta ao PHP se aceita name=value: roda com `-d name=value` e
// confere que ini_get não devolve false, que é como o PHP diz "diretiva
// desconhecida" (o -d em si não reclama de nome inexistente).
func PHPIniKnows(ctx context.Context, inst Installed, ext []string, name, value string) (bool, error) {
	if !ValidIniName(name) {
		return false, fmt.Errorf("runtime: nome de diretiva inválido %q", name)
	}
	if strings.ContainsAny(value, "\r\n") {
		return false, fmt.Errorf("runtime: valor de diretiva com quebra de linha")
	}
	args := append(extensionArgs(inst, ext), "-d", name+"="+value,
		"-r", "echo PHP_EOL.'"+iniMarker+"'.json_encode(ini_get('"+name+"')!==false);")
	raw, err := runIniScript(ctx, inst, args)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(raw)) == "true", nil
}

// extensionArgs monta -n e os -d que carregam as extensões da série, só as que
// têm arquivo em inst.ExtDir (mesma regra do render.RenderPHPIni: módulo
// ausente vira warning).
func extensionArgs(inst Installed, ext []string) []string {
	extDir := inst.ExtDir
	args := []string{"-n", "-d", "extension_dir=" + filepath.ToSlash(extDir)}
	for _, name := range ext {
		if name == "opcache" || !isFile(filepath.Join(extDir, extFile(name))) {
			continue
		}
		args = append(args, "-d", "extension="+name)
	}
	if opcache := extFile("opcache"); isFile(filepath.Join(extDir, opcache)) {
		args = append(args, "-d", "zend_extension="+opcache)
	}
	return args
}

// runIniScript roda o php.exe da série e devolve o que vem depois do marcador.
func runIniScript(ctx context.Context, inst Installed, args []string) ([]byte, error) {
	out, err := run(ctx, inst.Dir, inst.Exe, args...)
	if err != nil {
		return nil, err
	}
	i := strings.LastIndex(out, iniMarker)
	if i < 0 {
		return nil, fmt.Errorf("runtime: saída inesperada do php: %q", strings.TrimSpace(out))
	}
	return []byte(strings.TrimSpace(out[i+len(iniMarker):])), nil
}

func isFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
