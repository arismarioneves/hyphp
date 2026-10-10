package netcfg

import (
	"bufio"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// bindOccupied separa porta ocupada de bind recusado por permissão. Sem root,
// o macOS só deixa escutar abaixo de 1024 no endereço curinga, que é como o
// Apache e o nginx do HyPHP escutam; o bind de teste em 127.0.0.1:80 dá EACCES
// com a porta livre e virava o aviso falso "porta 80 em uso". Com o EACCES
// fora da conta, quem decide é o bind no curinga e o lsof em seguida.
func bindOccupied(err error) bool { return !errors.Is(err, syscall.EACCES) }

// listeningPorts executa `lsof -nP -iTCP -sTCP:LISTEN -F pcn` e devolve porta → PID de
// todo listener TCP (v4 e v6). -n/-P evitam DNS e nomes de serviço: a saída fica numérica
// e rápida. Não precisa de sysproc.Hide: no macOS não há janela de console para esconder.
func listeningPorts() (map[int]int, error) {
	out, err := exec.Command("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-F", "pcn").Output()
	if err != nil {
		// O lsof sai com código 1 em dois casos que não são falha: nenhum socket casou
		// com o filtro (stdout vazio, "nenhum listener") e algum descritor que ele não
		// conseguiu inspecionar — processo de outro usuário, comum sem root — enquanto
		// os registros que conseguiu ler saem completos no stdout. Descartá-los deixaria
		// o alocador sem enxergar portas ocupadas. Outro código (ou morte por sinal) é
		// falha de verdade e a saída parcial não é confiável.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return nil, fmt.Errorf("lsof: %w", err)
		}
	}
	return parseLsofListening(string(out)), nil
}

// parseLsofListening extrai porta → PID da saída `-F pcn` do lsof. Cada processo abre um
// bloco com "p<PID>"; os sockets dele vêm depois como "n<endereço>:<porta>" ("*:80",
// "[::]:80", "127.0.0.1:9000"). Linhas "c" (comando) e "f" (descritor) são ignoradas.
// A primeira ocorrência de cada porta vence (v4 e v6 do mesmo processo viram uma só).
func parseLsofListening(out string) map[int]int {
	res := make(map[int]int)
	pid := 0
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			// PID ilegível zera o bloco para não atribuir sockets ao processo anterior.
			n, err := strconv.Atoi(line[1:])
			if err != nil {
				n = 0
			}
			pid = n
		case 'n':
			if pid <= 0 {
				continue
			}
			colon := strings.LastIndexByte(line, ':')
			if colon < 0 {
				continue
			}
			port, err := strconv.Atoi(line[colon+1:])
			if err != nil {
				continue
			}
			if _, seen := res[port]; !seen {
				res[port] = pid
			}
		}
	}
	return res
}

// processImagePath devolve o executável do PID via `ps -o comm= -p <pid>`; no macOS o
// campo comm traz o caminho completo do binário.
func processImagePath(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("pid inválido: %d", pid)
	}
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", fmt.Errorf("ps -p %d: %w", pid, err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("ps -p %d: sem executável", pid)
	}
	return path, nil
}
