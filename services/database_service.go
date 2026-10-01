package services

import (
	"context"
	"fmt"
	"log/slog"

	"hyphp/internal/i18n"
	"hyphp/internal/mysqlcli"
	"hyphp/internal/runtime"
	"hyphp/internal/stack"
	"hyphp/internal/supervisor"
)

// DBInfo é a linha da tela Banco (C13).
type DBInfo struct {
	Name   string  `json:"name"`
	SizeMB float64 `json:"sizeMb"`
}

// Credentials é o que a UI mostra para montar a string de conexão (C18.7).
// É struct, e não múltiplos retornos, porque o gerador de bindings TS não
// representa mais de um valor não-erro.
type Credentials struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	// Engine é o motor no ar ("mysql" ou "mariadb", vazio sem banco) e
	// Command a linha para abrir o cliente dele num terminal, com o caminho
	// completo (mysqlcli.Client.Command); vazio sem banco instalado.
	Engine  string `json:"engine"`
	Command string `json:"command"`
}

// ErrMySQLIndisponivel é devolvido quando não há MySQL instalado ou o serviço
// não está pronto. Sem ele a UI mostraria o erro cru de conexão do cliente.
//
// É um tipo, e não errors.New, porque a variável nasce no init — antes de o
// idioma ser escolhido — e o texto precisa sair no idioma da hora em que o
// erro é mostrado. O valor é comparável, então errors.Is segue funcionando.
var ErrMySQLIndisponivel error = mysqlIndisponivel{}

type mysqlIndisponivel struct{}

func (mysqlIndisponivel) Error() string { return i18n.T("err.db.mysqlDown") }

type DatabaseDeps struct {
	Sup      *supervisor.Supervisor
	Stack    *stack.Stack
	Runtimes func() []runtime.Installed
	Logger   *slog.Logger
}

type DatabaseService struct {
	sup      *supervisor.Supervisor
	stk      *stack.Stack
	runtimes func() []runtime.Installed
	log      *slog.Logger
}

func NewDatabaseService(d DatabaseDeps) *DatabaseService {
	return &DatabaseService{sup: d.Sup, stk: d.Stack, runtimes: d.Runtimes, log: d.Logger}
}

// Status devolve o status do spec mysql. Se ele não existe (MySQL não
// instalado), devolve um status stopped em vez de zero value — a UI precisa do
// ID e do nome para desenhar a tela vazia.
func (d *DatabaseService) Status() supervisor.Status {
	if st, ok := d.sup.Status(stack.MySQLSpecID); ok {
		return st
	}
	return supervisor.Status{ID: stack.MySQLSpecID, Name: "MySQL", Group: "db", State: supervisor.Stopped}
}

// Databases lista os schemas de usuário com tamanho em MB.
func (d *DatabaseService) Databases() ([]DBInfo, error) {
	c, err := d.client()
	if err != nil {
		return nil, err
	}
	dbs, err := c.Databases(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]DBInfo, 0, len(dbs))
	for _, db := range dbs {
		out = append(out, DBInfo{Name: db.Name, SizeMB: db.SizeMB})
	}
	return out, nil
}

// Create cria um database vazio em utf8mb4. A validação de nome é a mesma do
// Stack (mysqlcli.ValidateName), chamada dentro de Create.
func (d *DatabaseService) Create(name string) error {
	c, err := d.client()
	if err != nil {
		return err
	}
	if err := c.Create(context.Background(), name); err != nil {
		return err
	}
	d.log.Info("services: database criado", "name", name)
	return nil
}

// Drop remove um database de usuário; schemas de sistema são recusados pelo
// mysqlcli.
func (d *DatabaseService) Drop(name string) error {
	c, err := d.client()
	if err != nil {
		return err
	}
	if err := c.Drop(context.Background(), name); err != nil {
		return err
	}
	d.log.Info("services: database removido", "name", name)
	return nil
}

// Credentials é o acesso do servidor embutido: root sem senha em 127.0.0.1,
// resultado do --initialize-insecure. É ambiente de desenvolvimento escutando
// só em loopback; a senha vazia é decisão registrada, não descuido.
func (d *DatabaseService) Credentials() Credentials {
	c := Credentials{User: "root", Password: "", Host: "127.0.0.1", Port: d.stk.State().MySQLPort}
	if inst, ok := stack.DBRuntime(d.runtimes(), d.stk.State()); ok {
		c.Engine = string(inst.Kind)
		c.Command = mysqlcli.New(inst, c.Port).Command(c.Password)
	}
	return c
}

// PhpMyAdminURL é a URL do vhost dedicado da ferramenta, ou "" quando ela não
// está instalada. É "" em vez de erro porque a ausência não é falha: a UI usa
// a string vazia para oferecer a instalação no lugar do botão de abrir.
func (d *DatabaseService) PhpMyAdminURL() string {
	if len(runtime.ByKind(d.runtimes(), runtime.PhpMyAdmin)) == 0 {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d", d.stk.State().PhpMyAdminPort)
}

// client exige o servidor de banco instalado e ready.
func (d *DatabaseService) client() (mysqlcli.Client, error) {
	// O mesmo servidor que a stack sobe (motor e versão): o cliente de outro
	// até conectaria, mas mysqldump e afins têm de casar com o servidor.
	inst, ok := stack.DBRuntime(d.runtimes(), d.stk.State())
	if !ok {
		return mysqlcli.Client{}, ErrMySQLIndisponivel
	}
	if st, ok := d.sup.Status(stack.MySQLSpecID); !ok || st.State != supervisor.Ready {
		return mysqlcli.Client{}, ErrMySQLIndisponivel
	}
	return mysqlcli.New(inst, d.stk.State().MySQLPort), nil
}
