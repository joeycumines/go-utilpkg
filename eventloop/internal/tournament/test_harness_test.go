package tournament

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type tournamentTestBarrier struct {
	done      chan struct{}
	remaining atomic.Int64
}

func newTournamentTestBarrier(count int) *tournamentTestBarrier {
	barrier := &tournamentTestBarrier{done: make(chan struct{})}
	barrier.remaining.Store(int64(count))
	if count == 0 {
		close(barrier.done)
	}
	return barrier
}

func (b *tournamentTestBarrier) Done() {
	remaining := b.remaining.Add(-1)
	if remaining == 0 {
		close(b.done)
	} else if remaining < 0 {
		panic("tournament test barrier completed too many times")
	}
}

func startTournamentTestLoop(t *testing.T, impl Implementation) (EventLoop, func()) {
	t.Helper()
	loop, err := impl.Factory()
	if err != nil {
		t.Fatalf("create %s loop: %v", impl.VariantID, err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	cleanup := benchmarkLoopCleanup(t, loop, runDone, impl.VariantID)
	t.Cleanup(cleanup)
	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		t.Fatalf("warmup %s loop: %v", impl.VariantID, err)
	}
	waitTournamentSignal(t, ready, "loop warmup")
	return loop, cleanup
}

func waitTournamentCount(t *testing.T, signal <-chan struct{}, count int, operation string) {
	t.Helper()
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for completed := range count {
		select {
		case <-signal:
		case <-timer.C:
			t.Fatalf("%s timed out after %d of %d completions", operation, completed, count)
		}
	}
}

func waitTournamentSignal(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(30 * time.Second):
		t.Fatalf("%s timed out", operation)
	}
}
