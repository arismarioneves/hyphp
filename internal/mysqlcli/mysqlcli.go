// Package mysqlcli conversa com o MySQL pela linha de comando (mysql.exe).
//
// Não há driver Go no projeto: o cliente oficial já está em disco junto do
// servidor, fala a versão certa do protocolo e evita uma dependência nova para
// três comandos DDL e uma consulta. O preço é parsear TSV — por isso o parser
// é isolado e testado.
package mysqlcli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"hyphp/internal/runtime"
)

// cmdTimeout cobre qualquer comando do cliente.
const cmdTimeout = 20 * time.Second

// systemSchemas nunca aparecem na UI nem podem ser removidos.
var systemSchemas = map[string]bool{
	"mysql":              true,
	"information_schema": true,
	"performance_schema": true,
	"sys":                true,
}

// IsSystemSchema informa se o nome é um schema interno do servidor.
func IsSystemSchema(name string) bool { return systemSchemas[strings.ToLower(name)] }

// nameRe é a única porta de entrada de nomes de database em SQL. O nome vem de
// hyphp.yaml — arquivo de terceiro, editável à mão — e é interpolado entre
// crases; sem esta validação, `a`; DROP DATABASE `b` seria executado.
var nameRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

// ValidateName aceita apenas [A-Za-z0-9_] com 1 a 64 caracteres (64 é o limite
// de identificador do MySQL).
func ValidateName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("mysqlcli: nome de database inválido %q (use 1 a 64 caracteres em [A-Za-z0-9_])", name)
	}
	return nil
}

// Client é um cliente sem estado; cada chamada roda o cliente de linha de
// comando do servidor (mysql.exe ou mariadb.exe).
type Client struct {
	Exe  string // <inst.Dir>/bin/mysql.exe ou mariadb.exe
	Host string
	Port int
	User string
}

// New monta o cliente a partir do servidor de banco detectado (MySQL ou
// MariaDB). root sem senha é o resultado da inicialização dos dois motores
// (--initialize-insecure e mariadb-install-db); o servidor só escuta em
// loopback.
func New(inst runtime.Installed, port int) Client {
	return Client{
		Exe:  ClientExe(inst),
		Host: "127.0.0.1",
		Port: port,
		User: "root",
	}
}

// ClientExe é o cliente de linha de comando que casa com o servidor. No
// MariaDB é o mariadb.exe: o mysql.exe do mesmo zip é o mesmo programa com o
// nome antigo, que o MariaDB vem aposentando. Um cliente do outro motor não
// serve: o do MariaDB não traz o caching_sha2_password que o root do MySQL
// 8.4 usa.
func ClientExe(inst runtime.Installed) string {
	exe := "mysql.exe"
	if inst.Kind == runtime.MariaDB {
		exe = "mariadb.exe"
	}
	return filepath.Join(inst.Dir, "bin", exe)
}

// Command é a linha para abrir o cliente num terminal, com o caminho
// completo: o HyPHP não põe os clientes no PATH, e o `mysql` que estiver lá
// pode ser de outra instalação (Laragon, XAMPP) e de outra versão.
//
// Cada valor vai separado da opção (`-h 127.0.0.1`): o PowerShell, o shell
// padrão do Windows Terminal, parte `-h127.0.0.1` no primeiro ponto e o
// cliente tenta o host "127". A senha é a exceção: `-p senha` com espaço faz
// o cliente pedir a senha e tomar "senha" por database. Caminho com espaço
// vai entre aspas, a forma do cmd; no PowerShell ela precisa de `&` na frente.
func (c Client) Command(password string) string {
	exe := c.Exe
	if strings.ContainsRune(exe, ' ') {
		exe = `"` + exe + `"`
	}
	cmd := fmt.Sprintf("%s -u %s", exe, c.User)
	if password != "" {
		cmd += " -p" + password
	}
	return fmt.Sprintf("%s -h %s -P %d", cmd, c.Host, c.Port)
}

// Create cria o database se não existir, em utf8mb4.
func (c Client) Create(ctx context.Context, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	sql := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", name)
	if _, err := c.query(ctx, sql); err != nil {
		return fmt.Errorf("mysqlcli: criar database %s: %w", name, err)
	}
	return nil
}

// Drop remove o database. Recusa schemas de sistema.
func (c Client) Drop(ctx context.Context, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if IsSystemSchema(name) {
		return fmt.Errorf("mysqlcli: %s é um schema do servidor e não pode ser removido", name)
	}
	if _, err := c.query(ctx, fmt.Sprintf("DROP DATABASE `%s`", name)); err != nil {
		return fmt.Errorf("mysqlcli: remover database %s: %w", name, err)
	}
	return nil
}

// query roda uma sentença e devolve a saída em TSV sem cabeçalho (-N sem nomes
// de coluna, -B em modo batch). --protocol=TCP é obrigatório: sem ele o cliente
// tenta named pipe quando o host é local e falha com "Can't open named pipe".
func (c Client) query(ctx context.Context, sql string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	args := []string{
		"-u" + c.User,
		"--host=" + c.Host,
		fmt.Sprintf("--port=%d", c.Port),
		"--protocol=TCP",
		"-N", "-B",
		"-e", sql,
	}
	cmd := exec.CommandContext(ctx, c.Exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	// Resultado só do stdout: o cliente do MariaDB 11.4+ avisa no stderr
	// ("--ssl-verify-server-cert is disabled ... passwordless login") a cada
	// chamada, e com a saída combinada o aviso virava um database na lista.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("mysqlcli: %s: %w: %s", filepath.Base(c.Exe), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// DB é um schema de usuário com o tamanho somado de dados e índices.
type DB struct {
	Name   string
	SizeMB float64
}

// listQuery lista TODOS os schemas, inclusive os vazios.
//
// A consulta óbvia (GROUP BY table_schema em information_schema.tables) só
// enxerga schema que já tem tabela: um database recém-criado sumiria da lista
// logo depois de o usuário criá-lo. information_schema.schemata é a mesma
// fonte que o SHOW DATABASES usa, e o LEFT JOIN traz o tamanho na mesma ida.
const listQuery = `SELECT s.SCHEMA_NAME, ROUND(COALESCE(SUM(t.DATA_LENGTH + t.INDEX_LENGTH), 0) / 1048576, 2) ` +
	`FROM information_schema.SCHEMATA s ` +
	`LEFT JOIN information_schema.TABLES t ON t.TABLE_SCHEMA = s.SCHEMA_NAME ` +
	`GROUP BY s.SCHEMA_NAME ORDER BY s.SCHEMA_NAME`

// Databases devolve os schemas de usuário com o tamanho em MB.
func (c Client) Databases(ctx context.Context) ([]DB, error) {
	out, err := c.query(ctx, listQuery)
	if err != nil {
		return nil, err
	}
	return parseDatabases(out), nil
}

// parseDatabases interpreta a saída TSV de listQuery: "<schema>\t<mb>" por
// linha. Schemas de sistema são descartados; tamanho ausente ou NULL vira 0.
func parseDatabases(out string) []DB {
	var dbs []DB
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		name := strings.TrimSpace(fields[0])
		if name == "" || IsSystemSchema(name) {
			continue
		}
		size := 0.0
		if len(fields) > 1 {
			raw := strings.TrimSpace(fields[1])
			if raw != "" && !strings.EqualFold(raw, "NULL") {
				if v, err := strconv.ParseFloat(raw, 64); err == nil {
					size = v
				}
			}
		}
		dbs = append(dbs, DB{Name: name, SizeMB: size})
	}
	return dbs
}
