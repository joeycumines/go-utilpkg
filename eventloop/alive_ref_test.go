package eventloop

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestAlive_NoWork tests that Alive() returns false when no work is pending.
func TestAlive_NoWork(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	if loop.Alive() {
		t.Error("Alive() should return false when no work is pending")
	}
}

// TestAlive_TimerPending tests that Alive() returns true when a timer is pending.
func TestAlive_TimerPending(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	_, err = loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Wait for the timer to be registered (SubmitInternal processes it)
	time.Sleep(20 * time.Millisecond)

	if !loop.Alive() {
		t.Error("Alive() should return true when a timer is pending")
	}
}

// TestAlive_ExternalTask tests that Alive() returns true when external tasks are queued.
func TestAlive_ExternalTask(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Block the loop so tasks accumulate
	blockDone := make(chan struct{})
	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Submit a task that blocks the loop
	err = loop.Submit(func() {
		<-blockDone
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// Now submit another task — it should be in the queue
	err = loop.Submit(func() {})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Alive should see the queued task
	if !loop.Alive() {
		t.Error("Alive() should return true when external tasks are queued")
	}

	close(blockDone)
}

// TestAlive_PromisifyGoroutine tests that Alive() returns true when Promisify goroutines are in-flight.
func TestAlive_PromisifyGoroutine(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Start a long-running Promisify goroutine
	unblock := make(chan struct{})
	p := loop.Promisify(ctx, func(ctx context.Context) (any, error) {
		<-unblock
		return "done", nil
	})
	_ = p

	// Give the goroutine time to start
	time.Sleep(20 * time.Millisecond)

	if !loop.Alive() {
		t.Error("Alive() should return true when Promisify goroutine is in-flight")
	}

	// Unblock the goroutine and wait for drain
	close(unblock)
	time.Sleep(50 * time.Millisecond)
}

// TestRefTimer_UnrefTimer basic state transitions.
func TestRefTimer_UnrefTimer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Schedule a timer
	id, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	// Wait for the timer to be registered
	time.Sleep(20 * time.Millisecond)

	if !loop.Alive() {
		t.Fatal("Alive() should return true with a pending timer")
	}

	// Unref the timer
	if err := loop.UnrefTimer(id); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	if loop.Alive() {
		t.Error("Alive() should return false after UnrefTimer (no ref'd work)")
	}

	// Ref the timer again
	if err := loop.RefTimer(id); err != nil {
		t.Fatalf("RefTimer: %v", err)
	}

	if !loop.Alive() {
		t.Error("Alive() should return true after RefTimer")
	}
}

// TestUnrefTimer_AllowsExit tests that an unref'd interval doesn't keep the loop alive.
func TestUnrefTimer_AllowsExit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Schedule an interval that would normally run forever
	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS: %v", err)
	}

	intervalID, err := js.SetInterval(func() {}, 100)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}
	// Wait for the timer to be registered
	time.Sleep(20 * time.Millisecond)

	if !loop.Alive() {
		t.Fatal("Alive() should return true with a ref'd interval")
	}

	// Unref the interval using the JS-level API
	if err := js.UnrefInterval(intervalID); err != nil {
		t.Fatalf("UnrefInterval: %v", err)
	}

	if loop.Alive() {
		t.Error("Alive() should return false after unref'ing the only timer")
	}
}

// TestRefTimer_NonexistentTimer tests that ref/unref silently ignore nonexistent timers.
func TestRefTimer_NonexistentTimer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Ref/unref a timer that doesn't exist — should not panic
	if err := loop.RefTimer(99999); err != nil {
		t.Fatalf("RefTimer nonexistent: %v", err)
	}
	if err := loop.UnrefTimer(99999); err != nil {
		t.Fatalf("UnrefTimer nonexistent: %v", err)
	}
}

// TestRefTimer_DoubleRef tests that double-ref is idempotent.
func TestRefTimer_DoubleRef(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	id, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	// Double ref — should be idempotent (count stays at 1)
	if err := loop.RefTimer(id); err != nil {
		t.Fatalf("RefTimer: %v", err)
	}
	if err := loop.RefTimer(id); err != nil {
		t.Fatalf("RefTimer: %v", err)
	}

	// Single unref should make it dead
	if err := loop.UnrefTimer(id); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	if loop.Alive() {
		t.Error("Alive() should return false after one unref (double ref is idempotent)")
	}
}

// TestRefTimer_FromExternalGoroutine tests ref/unref from a non-loop goroutine.
func TestRefTimer_FromExternalGoroutine(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	id, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	// Unref from external goroutine — sync.Map allows this
	if err := loop.UnrefTimer(id); err != nil {
		t.Fatalf("UnrefTimer from external: %v", err)
	}

	if loop.Alive() {
		t.Error("Alive() should return false after unref from external goroutine")
	}
}

// TestPromisifyCount_Tracking verifies promisifyCount is tracked correctly.
func TestPromisifyCount_Tracking(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	if loop.promisifyCount.Load() != 0 {
		t.Fatalf("promisifyCount should start at 0, got %d", loop.promisifyCount.Load())
	}

	_ = atomic.Int32{} // ensure atomic import used
	const n = 5
	for i := 0; i < n; i++ {
		p := loop.Promisify(ctx, func(ctx context.Context) (any, error) {
			time.Sleep(100 * time.Millisecond)
			return i, nil
		})
		_ = p
	}

	// Give goroutines time to start
	time.Sleep(20 * time.Millisecond)

	if loop.promisifyCount.Load() != n {
		t.Errorf("promisifyCount should be %d, got %d", n, loop.promisifyCount.Load())
	}

	if !loop.Alive() {
		t.Error("Alive() should return true with Promisify goroutines in-flight")
	}

	// Wait for all to finish
	time.Sleep(200 * time.Millisecond)
}

// TestAlive_SentinelDrainLoop tests the sentinel pattern end-to-end.
func TestAlive_SentinelDrainLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Schedule a short timer
	var executed atomic.Bool
	_, err = loop.ScheduleTimer(10*time.Millisecond, func() {
		executed.Store(true)
	})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Run the sentinel drain loop using SubmitInternal (not Submit).
	// SubmitInternal sends to the internal queue, which is the same queue
	// used by ScheduleTimer for timer registrations. This guarantees FIFO
	// ordering: the timer registration is processed BEFORE the sentinel,
	// so Alive() accurately reflects the timer state when checked.
	// (Submit would go to auxJobs in fast path mode, which runs BEFORE
	// the internal queue in runAux(), causing premature Alive() == false.)
	for i := 0; i < 10000; i++ {
		done := make(chan struct{})
		if err := loop.SubmitInternal(func() { close(done) }); err != nil {
			break
		}
		<-done
		if !loop.Alive() {
			break
		}
	}

	if loop.Alive() {
		t.Error("Alive() should be false after timer fires and drains")
	}
	if !executed.Load() {
		t.Error("timer callback should have executed")
	}
}

// TestTimerCount_Tracking verifies that refedTimerCount tracks correctly
// through schedule, cancel, and unref operations.
func TestTimerCount_Tracking(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Schedule 3 timers (all ref'd by default)
	ids := make([]TimerID, 3)
	for i := range ids {
		ids[i], err = loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer %d: %v", i, err)
		}
	}
	time.Sleep(20 * time.Millisecond)

	if loop.refedTimerCount.Load() != 3 {
		t.Errorf("refedTimerCount should be 3, got %d", loop.refedTimerCount.Load())
	}

	// Cancel one — refedTimerCount decrements
	if err := loop.CancelTimer(ids[0]); err != nil {
		t.Fatalf("CancelTimer: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	if loop.refedTimerCount.Load() != 2 {
		t.Errorf("refedTimerCount should be 2 after cancel, got %d", loop.refedTimerCount.Load())
	}

	// Unref one — refedTimerCount decrements again
	if err := loop.UnrefTimer(ids[1]); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	if loop.refedTimerCount.Load() != 1 {
		t.Errorf("refedTimerCount should be 1 after unref, got %d", loop.refedTimerCount.Load())
	}
}

// TestCancelTimers_BatchCounterUpdate verifies CancelTimers updates counters correctly.
func TestCancelTimers_BatchCounterUpdate(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	ids := make([]TimerID, 3)
	for i := range ids {
		ids[i], err = loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer %d: %v", i, err)
		}
	}
	time.Sleep(20 * time.Millisecond)

	errors := loop.CancelTimers(ids)
	for i, e := range errors {
		if e != nil {
			t.Errorf("CancelTimers[%d]: %v", i, e)
		}
	}
	time.Sleep(20 * time.Millisecond)

	if loop.refedTimerCount.Load() != 0 {
		t.Errorf("refedTimerCount should be 0 after batch cancel, got %d", loop.refedTimerCount.Load())
	}
}
