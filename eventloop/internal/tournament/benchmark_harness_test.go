package tournament

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournamenttest"
)

type benchmarkBarrier struct {
	done      chan struct{}
	remaining atomic.Int64
}

type benchmarkProducerGroup struct {
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	wait     sync.WaitGroup
}

func newBenchmarkProducerGroup(tb testing.TB, count int) *benchmarkProducerGroup {
	tb.Helper()
	group := &benchmarkProducerGroup{stop: make(chan struct{}), done: make(chan struct{})}
	group.wait.Add(count)
	go func() {
		group.wait.Wait()
		close(group.done)
	}()
	tb.Cleanup(func() {
		group.Stop()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-group.done:
		case <-timer.C:
			tb.Errorf("benchmark producer cleanup timed out")
		}
	})
	return group
}

func (g *benchmarkProducerGroup) Go(worker func(<-chan struct{})) {
	go func() {
		defer g.wait.Done()
		worker(g.stop)
	}()
}

func (g *benchmarkProducerGroup) Stop() {
	g.stopOnce.Do(func() { close(g.stop) })
}

func (g *benchmarkProducerGroup) Done() <-chan struct{} {
	return g.done
}

func newBenchmarkBarrier(count int) *benchmarkBarrier {
	barrier := &benchmarkBarrier{done: make(chan struct{})}
	barrier.remaining.Store(int64(count))
	if count == 0 {
		close(barrier.done)
	}
	return barrier
}

func (b *benchmarkBarrier) Done() {
	remaining := b.remaining.Add(-1)
	if remaining == 0 {
		close(b.done)
	} else if remaining < 0 {
		panic("tournament benchmark barrier completed too many times")
	}
}

func startBenchmarkEventLoop(b *testing.B, impl Implementation) (EventLoop, func()) {
	b.Helper()
	loop, err := impl.Factory()
	if err != nil {
		b.Fatalf("create %s loop: %v", impl.VariantID, err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	cleanup := benchmarkLoopCleanup(b, loop, runDone, impl.VariantID)
	b.Cleanup(cleanup)
	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		b.Fatalf("warmup %s loop: %v", impl.VariantID, err)
	}
	waitBenchmarkSignal(b, ready, "loop warmup")

	return loop, cleanup
}

func benchmarkLoopCleanup(tb testing.TB, loop EventLoop, runDone <-chan error, label string) func() {
	tb.Helper()
	var cleanupOnce sync.Once
	return func() {
		cleanupOnce.Do(func() {
			result := tournamenttest.Terminate(loop, runDone, 5*time.Second)
			// "loop has been terminated" is expected when the loop was already
			// shut down by the test itself; it is not a cleanup error.
			if result.ShutdownErr != nil && !strings.Contains(result.ShutdownErr.Error(), "loop has been terminated") {
				tb.Errorf("shutdown %s loop: %v", label, result.ShutdownErr)
			}
			if result.CloseErr != nil && !strings.Contains(result.CloseErr.Error(), "loop has been terminated") {
				tb.Errorf("fallback close %s loop: %v", label, result.CloseErr)
			}
			if result.RunErr != nil {
				tb.Errorf("run %s loop: %v", label, result.RunErr)
			}
		})
	}
}

func waitBenchmarkSignal(b *testing.B, signal <-chan struct{}, operation string) {
	b.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		b.Fatalf("%s timed out", operation)
	}
}

func waitBenchmarkDeadline[T any](b *testing.B, signal <-chan T, deadline <-chan time.Time, operation string) {
	b.Helper()
	select {
	case <-signal:
	case <-deadline:
		b.Fatalf("%s timed out", operation)
	}
}

func waitBenchmarkBarrier(b *testing.B, barrier *benchmarkBarrier, deadline <-chan time.Time, operation string) {
	b.Helper()
	waitBenchmarkDeadline(b, barrier.done, deadline, operation)
}
