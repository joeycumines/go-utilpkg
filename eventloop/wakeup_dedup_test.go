package eventloop_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/eventlooptest"
)

// TestConcurrentSubmissionWakeIntegration complements the package-internal
// physical-epoch tests at the public API boundary. Every accepted callback must
// execute even when many producers race to publish a coalesced wake.
func TestConcurrentSubmissionWakeIntegration(t *testing.T) {
	loop := eventloop.New()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	t.Cleanup(func() {
		result := eventlooptest.Terminate(loop, runDone, 5*time.Second)
		if result.ShutdownErr != nil && !errors.Is(result.ShutdownErr, eventloop.ErrLoopTerminated) {
			t.Errorf("Shutdown: %v", result.ShutdownErr)
		}
		if result.CloseErr != nil && !errors.Is(result.CloseErr, eventloop.ErrLoopTerminated) {
			t.Errorf("fallback Close: %v", result.CloseErr)
		}
		if result.RunErr != nil {
			t.Errorf("Run: %v", result.RunErr)
		}
	})

	const producers = 100
	start := make(chan struct{})
	var submitted sync.WaitGroup
	var executed sync.WaitGroup
	submitted.Add(producers)
	executed.Add(producers)
	var executionCount atomic.Int32
	for range producers {
		go func() {
			defer submitted.Done()
			<-start
			if err := loop.Submit(func() {
				executionCount.Add(1)
				executed.Done()
			}); err != nil {
				t.Errorf("Submit: %v", err)
				executed.Done()
			}
		}()
	}
	close(start)
	submittedDone := make(chan struct{})
	go func() {
		submitted.Wait()
		close(submittedDone)
	}()
	select {
	case <-submittedDone:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent submissions did not all return")
	}
	done := make(chan struct{})
	go func() {
		executed.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("accepted concurrent submissions did not all execute")
	}
	if got := executionCount.Load(); got != producers {
		t.Fatalf("executed callbacks = %d, want %d", got, producers)
	}
}

// BenchmarkConcurrentSubmissionWakeIntegration measures the public concurrent
// submission path. Exact physical-write deduplication is tested with injected
// counters inside package eventloop rather than inferred from this benchmark.
func BenchmarkConcurrentSubmissionWakeIntegration(b *testing.B) {
	loop, cleanup := startConcurrentSubmissionBenchmarkLoop(b)
	defer cleanup()

	var executed atomic.Int64
	watchdog := time.AfterFunc(30*time.Minute, func() {
		panic("BenchmarkConcurrentSubmissionWakeIntegration timed out")
	})
	defer watchdog.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := loop.Submit(func() { executed.Add(1) }); err != nil {
				b.Errorf("Submit: %v", err)
			}
		}
	})
	b.StopTimer()

	drained := make(chan struct{})
	if err := loop.Submit(func() { close(drained) }); err != nil {
		b.Fatalf("Submit drain barrier: %v", err)
	}
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		b.Fatal("accepted callbacks did not drain")
	}
	if got := executed.Load(); got != int64(b.N) {
		b.Fatalf("executed callbacks = %d, want %d", got, b.N)
	}
}

func startConcurrentSubmissionBenchmarkLoop(b *testing.B) (*eventloop.Loop, func()) {
	b.Helper()
	loop := eventloop.New()
	runDone := make(chan error, 1)
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			result := eventlooptest.Terminate(loop, runDone, 5*time.Second)
			if result.ShutdownErr != nil && !errors.Is(result.ShutdownErr, eventloop.ErrLoopTerminated) {
				b.Errorf("Shutdown: %v", result.ShutdownErr)
			}
			if result.CloseErr != nil && !errors.Is(result.CloseErr, eventloop.ErrLoopTerminated) {
				b.Errorf("fallback Close: %v", result.CloseErr)
			}
			if result.RunErr != nil {
				b.Errorf("Run: %v", result.RunErr)
			}
		})
	}
	b.Cleanup(cleanup)
	go func() { runDone <- loop.Run(context.Background()) }()
	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		b.Fatalf("warmup Submit: %v", err)
	}
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		b.Fatal("loop warmup timed out")
	}

	return loop, cleanup
}
