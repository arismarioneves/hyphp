package supervisor

import (
	"math"
	"testing"
	"time"
)

func TestNextDelay(t *testing.T) {
	tests := []struct {
		name    string
		policy  RestartPolicy
		attempt int
		want    time.Duration
	}{
		{"attempt 0 devolve BaseDelay", RestartPolicy{BaseDelay: 100 * time.Millisecond, MaxDelay: time.Minute}, 0, 100 * time.Millisecond},
		{"primeira tentativa", RestartPolicy{BaseDelay: 100 * time.Millisecond, MaxDelay: time.Minute}, 1, 100 * time.Millisecond},
		{"dobra na segunda", RestartPolicy{BaseDelay: 100 * time.Millisecond, MaxDelay: time.Minute}, 2, 200 * time.Millisecond},
		{"quadruplica na terceira", RestartPolicy{BaseDelay: 100 * time.Millisecond, MaxDelay: time.Minute}, 3, 400 * time.Millisecond},
		{"satura no teto", RestartPolicy{BaseDelay: time.Second, MaxDelay: 3 * time.Second}, 9, 3 * time.Second},
		{"BaseDelay zero usa 1s", RestartPolicy{MaxDelay: time.Minute}, 1, time.Second},
		{"MaxDelay zero usa 30s", RestartPolicy{BaseDelay: time.Second}, 20, 30 * time.Second},
		{"MaxDelay menor que BaseDelay vira BaseDelay", RestartPolicy{BaseDelay: 10 * time.Second, MaxDelay: time.Second}, 1, 10 * time.Second},
		{"attempt gigante não estoura", RestartPolicy{BaseDelay: time.Second, MaxDelay: 30 * time.Second}, math.MaxInt32, 30 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextDelay(tc.policy, tc.attempt); got != tc.want {
				t.Fatalf("nextDelay(%+v, %d) = %s, want %s", tc.policy, tc.attempt, got, tc.want)
			}
		})
	}
}
