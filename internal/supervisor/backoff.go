package supervisor

import "time"

const (
	fallbackBaseDelay = time.Second
	fallbackMaxDelay  = 30 * time.Second
)

// nextDelay devolve quanto esperar antes da tentativa `attempt` de reinício
// (1 = primeira). Cresce exponencialmente a partir de BaseDelay e satura em
// MaxDelay. Nunca estoura int64: a duplicação para assim que o próximo passo
// ultrapassaria o teto, então o laço roda no máximo ~63 vezes.
func nextDelay(p RestartPolicy, attempt int) time.Duration {
	base := p.BaseDelay
	if base <= 0 {
		base = fallbackBaseDelay
	}
	maxDelay := p.MaxDelay
	if maxDelay <= 0 {
		maxDelay = fallbackMaxDelay
	}
	if maxDelay < base {
		maxDelay = base
	}
	if attempt <= 1 {
		return base
	}
	d := base
	for range attempt - 1 {
		if d > maxDelay/2 {
			return maxDelay
		}
		d *= 2
	}
	if d > maxDelay {
		return maxDelay
	}
	return d
}
