package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"hyphp/internal/project"
	"hyphp/internal/stack"
	"hyphp/internal/state"
)

// reconcileTimeout cobre render + validate + UAC do hosts.
const reconcileTimeout = 60 * time.Second

var phpMajorRe = regexp.MustCompile(`^\d+\.\d+$`)

// ProjectsService expõe projetos à UI. Toda mutação segue o mesmo caminho:
// grava (state.json ou hyphp.yaml) → rescan → SetProjects → project:changed →
// Reconcile. Serializado por mu: a UI pode disparar cliques em sequência.
type ProjectsService struct {
	mu      sync.Mutex
	app     *application.App
	stk     *stack.Stack
	watcher *project.Watcher // pode ser nil
	emit    func(name string, data any)
}

func NewProjectsService(app *application.App, stk *stack.Stack, watcher *project.Watcher, emit func(name string, data any)) *ProjectsService {
	return &ProjectsService{app: app, stk: stk, watcher: watcher, emit: emit}
}

// dialogCancelledMsg é a mensagem de cfd.ErrorCancelled, devolvida pelo
// diálogo do Windows quando o usuário fecha sem escolher. O pacote cfd é
// interno ao Wails (v3/internal/go-common-file-dialog), então não há sentinela
// importável para errors.Is — resta comparar a mensagem.
//
// Acoplamento a texto de terceiro, e portanto frágil: se um upgrade do Wails
// reescrever a mensagem, cancelar o diálogo passa a exibir um erro na tela de
// Projetos em vez de simplesmente não fazer nada. Esse é o sintoma a procurar;
// a correção é reconferir internal/go-common-file-dialog/cfd/errors.go.
const dialogCancelledMsg = "cancelled by user"

// PickRoot abre o diálogo nativo de seleção de pasta e devolve o caminho
// escolhido. Devolve "" quando o usuário cancela.
func (p *ProjectsService) PickRoot() (string, error) {
	dir, err := p.app.Dialog.OpenFile().
		SetTitle("Selecionar diretório de projetos").
		CanChooseDirectories(true).
		CanChooseFiles(false).
		PromptForSingleSelection()
	if err != nil {
		if strings.Contains(err.Error(), dialogCancelledMsg) {
			return "", nil
		}
		return "", fmt.Errorf("diálogo de pasta: %w", err)
	}
	return dir, nil
}

func (p *ProjectsService) List() ([]project.Project, error) {
	return p.stk.Projects(), nil
}

func (p *ProjectsService) Roots() []string {
	return p.stk.State().Roots
}

func (p *ProjectsService) AddRoot(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolver %q: %w", dir, err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("diretório %s: %w", abs, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("%s não é um diretório", abs)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	err = p.stk.UpdateState(func(s *state.State) {
		for _, r := range s.Roots {
			if filepath.Clean(r) == abs {
				return
			}
		}
		s.Roots = append(s.Roots, abs)
	})
	if err != nil {
		return err
	}
	return p.rescanLocked()
}

func (p *ProjectsService) RemoveRoot(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolver %q: %w", dir, err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	err = p.stk.UpdateState(func(s *state.State) {
		kept := s.Roots[:0]
		for _, r := range s.Roots {
			if filepath.Clean(r) != abs {
				kept = append(kept, r)
			}
		}
		s.Roots = kept
	})
	if err != nil {
		return err
	}
	return p.rescanLocked()
}

// Rescan redescobre projetos e reconcilia. Também é o callback do Watcher.
func (p *ProjectsService) Rescan() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rescanLocked()
}

// SetPHP grava `php: "<major>"` no hyphp.yaml do projeto (criando-o se não
// existir) e reconcilia. Não valida se a versão está instalada: o desired()
// avisa php-missing e a UI oferece o download.
func (p *ProjectsService) SetPHP(id, major string) error {
	if major != "" && !phpMajorRe.MatchString(major) {
		return fmt.Errorf("versão %q inválida; use major.minor (ex.: 8.1)", major)
	}
	return p.editManifest(id, func(m *project.Manifest) { m.PHP = major })
}

func (p *ProjectsService) SetWildcard(id string, on bool) error {
	return p.editManifest(id, func(m *project.Manifest) { m.Wildcard = on })
}

// CreateManifest materializa os defaults em hyphp.yaml. Idempotente.
func (p *ProjectsService) CreateManifest(id string) error {
	return p.editManifest(id, func(*project.Manifest) {})
}

func (p *ProjectsService) editManifest(id string, fn func(*project.Manifest)) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	cur, err := p.find(id)
	if err != nil {
		return err
	}
	// recarrega do disco: o yaml pode ter mudado desde o último scan
	fresh, err := project.Load(cur.Root)
	if err != nil {
		return err
	}
	fn(&fresh.Manifest)
	// Fixa a série em uso quando o manifesto ainda não a declara. O arquivo
	// descreve o ambiente: deixar `php` implícito faz quem clona rodar noutra
	// versão sem perceber, porque o padrão do HyPHP dele pode ser outro.
	if fresh.PHP == "" {
		fresh.PHP = cur.PHPEffective
	}
	if err := project.Validate(fresh.Manifest); err != nil {
		return err
	}
	if err := fresh.Write(); err != nil {
		return err
	}
	return p.rescanLocked()
}

func (p *ProjectsService) find(id string) (project.Project, error) {
	for _, pr := range p.stk.Projects() {
		if pr.ID == id {
			return pr, nil
		}
	}
	return project.Project{}, fmt.Errorf("projeto %q não encontrado", id)
}

// rescanLocked: Discover → SetProjects → watcher → project:changed → Reconcile.
// O erro do Reconcile é devolvido à UI, mas a lista de projetos já foi publicada.
func (p *ProjectsService) rescanLocked() error {
	roots := p.stk.State().Roots
	projs, err := project.Discover(roots)
	if err != nil {
		return err
	}
	p.stk.SetProjects(projs)
	if p.watcher != nil {
		_ = p.watcher.SetRoots(roots) // falha de watch não bloqueia; Rescan manual segue disponível
	}
	// A lista publicada tem de vir do Stack, não do Discover: só ela traz o
	// PHPEffective resolvido, e é ela que a UI usa para exibir a série.
	p.emit("project:changed", p.stk.Projects())

	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()
	if _, err := p.stk.Reconcile(ctx); err != nil {
		return fmt.Errorf("reconciliar: %w", err)
	}
	return nil
}
