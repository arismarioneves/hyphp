package supervisor

import "time"

// State é o estado observável de um serviço.
type State string

const (
	Stopped  State = "stopped"
	Starting State = "starting"
	Ready    State = "ready"
	Degraded State = "degraded"
	Stopping State = "stopping"
	Failed   State = "failed"
)

// RestartPolicy governa o reinício automático após saída inesperada.
type RestartPolicy struct {
	Enabled    bool
	MaxRetries int           // 0 = ilimitado
	BaseDelay  time.Duration // 0 = 1s
	MaxDelay   time.Duration // 0 = 30s
}

// Spec descreve um serviço supervisionado. Imutável depois de Add.
type Spec struct {
	ID            string // único e estável: "web:apache", "php:8.1:0", "mysql", "mailpit", "proc:<projeto>:<nome>"
	Name          string // exibição
	Group         string // "web" | "php" | "db" | "mail" | "proc"
	Exe           string
	Args          []string
	Env           []string // adicionais; herda os.Environ()
	Dir           string
	Port          int // informativo (0 se n/a)
	Probe         Probe
	ProbeInterval time.Duration // 0 → defaultProbeInterval
	ProbeTimeout  time.Duration // 0 → defaultProbeTimeout
	Restart       RestartPolicy
	LogPath       string // arquivo; "" = só ring
}

// Status é o retrato serializável de um serviço (payload de service:state).
type Status struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Group     string    `json:"group"`
	State     State     `json:"state"`
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	StartedAt time.Time `json:"startedAt"`
	Restarts  int       `json:"restarts"`
	LastError string    `json:"lastError"`
}

// Event é publicado em toda transição de estado.
type Event struct {
	Status Status
}

const (
	defaultProbeInterval = 500 * time.Millisecond
	defaultProbeTimeout  = 20 * time.Second
	readyProbeInterval   = 5 * time.Second
	readyProbeTimeout    = 3 * time.Second
	stopTimeout          = 5 * time.Second
	ringCapacity         = 2000
	eventBuffer          = 64
)
