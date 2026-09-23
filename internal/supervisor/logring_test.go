package supervisor

import (
	"fmt"
	"io"
	"reflect"
	"testing"
	"time"
)

func TestLogRing_OverflowKeepsLastN(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
		writes   int
		want     []string
	}{
		{"cabe tudo", 5, 2, []string{"l0", "l1"}},
		{"estoura e mantém as últimas", 3, 5, []string{"l2", "l3", "l4"}},
		{"capacidade 1", 1, 3, []string{"l2"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewLogRing(tc.capacity)
			for i := range tc.writes {
				fmt.Fprintf(r, "l%d\n", i)
			}
			if got := r.Lines(); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Lines() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLogRing_PartialWritesJoinIntoOneLine(t *testing.T) {
	r := NewLogRing(10)
	io.WriteString(r, "hel")
	io.WriteString(r, "lo\r\nwor")
	if got, want := r.Lines(), []string{"hello"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("antes do flush: Lines() = %v, want %v", got, want)
	}
	r.flush()
	if got, want := r.Lines(), []string{"hello", "wor"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("após flush: Lines() = %v, want %v", got, want)
	}
}

func TestLogRing_SubscribeReceivesOnlyNewLines(t *testing.T) {
	r := NewLogRing(10)
	io.WriteString(r, "old\n")
	ch, cancel := r.Subscribe()
	defer cancel()
	io.WriteString(r, "new\n")
	select {
	case got := <-ch:
		if got != "new" {
			t.Fatalf("recebeu %q, want %q", got, "new")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout esperando linha nova")
	}
	select {
	case extra := <-ch:
		t.Fatalf("linha inesperada %q (linhas antigas não devem ser replicadas)", extra)
	default:
	}
}

func TestLogRing_SlowSubscriberDropsInsteadOfBlocking(t *testing.T) {
	r := NewLogRing(10)
	_, cancel := r.Subscribe() // nunca lê
	defer cancel()
	const extra = 40
	done := make(chan struct{})
	go func() {
		for range subscriberBuffer + extra {
			io.WriteString(r, "x\n")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Write bloqueou com assinante lento")
	}
	if got := r.Dropped(); got != extra {
		t.Fatalf("Dropped() = %d, want %d", got, extra)
	}
}

func TestLogRing_CancelClosesChannelAndIsIdempotent(t *testing.T) {
	r := NewLogRing(2)
	ch, cancel := r.Subscribe()
	cancel()
	cancel()
	if _, ok := <-ch; ok {
		t.Fatal("canal deveria estar fechado após cancel")
	}
	io.WriteString(r, "after\n") // não pode entrar em pânico nem bloquear
	if got := r.Dropped(); got != 0 {
		t.Fatalf("Dropped() = %d após cancel, want 0", got)
	}
}
