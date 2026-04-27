package eventloop

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestAlive_AfterAllTimersFire exercises the refedTimerCount check (line 1455)
// transitioning from >0 to 0 after timers fire.
func TestAlive_AfterAllTimersFire(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	// Schedule a short timer
	_, err = loop.ScheduleTimer(5*time.Millisecond, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Wait for timer to fire and drain using sentinel pattern
	for i := 0; i < 10000; i++ {
		barrier := make(chan struct{})
		if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
			break
		}
		<-barrier
		if !loop.Alive() {
			break
		}
	}

	if loop.Alive() {
		t.Error("Alive() should return false after all timers have fired")
	}
}

// TestAlive_WithOnlyUnrefdTimers exercises that only refedTimerCount
// (line 1455) matters, not total timer count. A timer exists but is unref'd.
func TestAlive_WithOnlyUnrefdTimers(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	id, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Unref the timer — it still exists but doesn't count
	if err := loop.UnrefTimer(id); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	if loop.Alive() {
		t.Error("Alive() should return false with only unref'd timers")
	}
}

// TestAlive_PromisifyDuringExecution exercises Alive() from within a Promisify
// callback (line 1473: promisifyCount > 0). Verifies no deadlock and returns true.
func TestAlive_PromisifyDuringExecution(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	var aliveResult atomic.Bool
	done := make(chan struct{})

	// Start a Promisify goroutine
	_ = loop.Promisify(ctx, func(ctx context.Context) (any, error) {
		// Call Alive() from within the Promisify callback
		aliveResult.Store(loop.Alive())
		close(done)
		return nil, nil
	})

	// Wait for completion
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Promisify callback")
	}

	if !aliveResult.Load() {
		t.Error("Alive() should return true from within a Promisify callback (promisifyCount > 0)")
	}
}

// TestAlive_DuringShutdown exercises Alive() after the loop has fully
// terminated. All queues drained, all counters zeroed.
func TestAlive_DuringShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go loop.Run(ctx)

	// Schedule and wait for a timer
	_, err = loop.ScheduleTimer(5*time.Millisecond, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	// Shutdown the loop
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// After shutdown, Alive() should return false
	if loop.Alive() {
		t.Error("Alive() should return false after loop shutdown")
	}
}

// TestAlive_ExternalAndInternalQueues exercises lines 1458-1468:
// both the internal queue and external queue contain pending work.
func TestAlive_ExternalAndInternalQueues(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Block the loop to accumulate work
	blockDone := make(chan struct{})
	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Submit a blocking callback
	err = loop.Submit(func() {
		<-blockDone
	})
	if err != nil {
		t.Fatalf("Submit blocking: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// Now submit via Submit (external queue)
	err = loop.Submit(func() {})
	if err != nil {
		t.Fatalf("Submit external: %v", err)
	}

	// Also submit via SubmitInternal (internal queue)
	err = loop.SubmitInternal(func() {})
	if err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}

	// Alive should see the queued work
	if !loop.Alive() {
		t.Error("Alive() should return true with both external and internal queues populated")
	}

	close(blockDone)
}

// TestAlive_MicrotaskOnly exercises line 1470 (microtasks.IsEmpty()):
// Alive() returns true when only microtasks are pending.
func TestAlive_MicrotaskOnly(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Block the loop so microtasks accumulate
	blockDone := make(chan struct{})
	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	err = loop.Submit(func() {
		<-blockDone
	})
	if err != nil {
		t.Fatalf("Submit blocking: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// Schedule a microtask while loop is blocked
	err = loop.ScheduleMicrotask(func() {})
	if err != nil {
		t.Fatalf("ScheduleMicrotask: %v", err)
	}

	// Alive should see the pending microtask
	if !loop.Alive() {
		t.Error("Alive() should return true with pending microtasks")
	}

	close(blockDone)
}

// TestAlive_AuxJobsOnly exercises line 1465 (len(l.auxJobs) > 0):
// Alive() returns true when only auxJobs are pending.
func TestAlive_AuxJobsOnly(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Block the loop to accumulate auxJobs
	blockDone := make(chan struct{})
	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Use Submit to put work into the external queue
	err = loop.Submit(func() {
		<-blockDone
	})
	if err != nil {
		t.Fatalf("Submit blocking: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// Submit more work — these go to auxJobs in fast-path mode
	// when the loop is busy processing the blocking callback
	err = loop.Submit(func() {})
	if err != nil {
		t.Fatalf("Submit aux: %v", err)
	}

	// Alive should see the queued work (external or auxJobs)
	if !loop.Alive() {
		t.Error("Alive() should return true with pending auxJobs")
	}

	close(blockDone)
}
