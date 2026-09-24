package services

import (
	"context"
	"errors"
	"log/slog"

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
}

// ErrMySQLIndisponivel é devolvido quando não há MySQL instalado ou o serviço
// não está pronto. Sem ele a UI mostraria o erro cru de conexão do cliente.
var ErrMySQLIndisponivel = errors.New("services: MySQL não está rodando")

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
	return Credentials{User: "root", Password: "", Host: "127.0.0.1", Port: d.stk.State().MySQLPort}
}

// client exige MySQL instalado e ready.
func (d *DatabaseService) client() (mysqlcli.Client, error) {
	list := runtime.ByKind(d.runtimes(), runtime.MySQL)
	if len(list) == 0 {
		return mysqlcli.Client{}, ErrMySQLIndisponivel
	}
	if st, ok := d.sup.Status(stack.MySQLSpecID); !ok || st.State != supervisor.Ready {
		return mysqlcli.Client{}, ErrMySQLIndisponivel
	}
	return mysqlcli.New(list[0], d.stk.State().MySQLPort), nil
}
