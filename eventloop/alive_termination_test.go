package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestAlive_ShutdownClearsPendingTimer verifies that Shutdown clears pending
// timer liveness so Alive() reports false after termination even when a timer
// would otherwise still be pending far in the future.
func TestAlive_ShutdownClearsPendingTimer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = loop.Run(ctx) }()
	waitForLoopRunning(t, loop, 2*time.Second)

	_, err = loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	if !loop.Alive() {
		t.Fatal("Alive() should return true before shutdown with a pending ref'd timer")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if loop.Alive() {
		t.Fatal("Alive() should return false after shutdown with pending timer state cleared")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount should be 0 after shutdown, got %d", got)
	}
	if got := len(loop.timers); got != 0 {
		t.Fatalf("timers heap should be empty after shutdown, got %d entries", got)
	}
	if got := len(loop.timerMap); got != 0 {
		t.Fatalf("timerMap should be empty after shutdown, got %d entries", got)
	}
}

// TestAlive_CloseClearsPendingTimerAndFD verifies that Close clears all
// remaining liveness signals for the running-loop path, including pending
// timers and registered user I/O FDs.
func TestAlive_CloseClearsPendingTimerAndFD(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = loop.Run(ctx) }()
	waitForLoopRunning(t, loop, 2*time.Second)

	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	_, err = loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	if !loop.Alive() {
		t.Fatal("Alive() should return true before Close with pending timer and registered FD")
	}

	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if loop.Alive() {
		t.Fatal("Alive() should return false after Close with pending timer and FD state cleared")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount should be 0 after Close, got %d", got)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount should be 0 after Close, got %d", got)
	}
	if got := len(loop.timers); got != 0 {
		t.Fatalf("timers heap should be empty after Close, got %d entries", got)
	}
	if got := len(loop.timerMap); got != 0 {
		t.Fatalf("timerMap should be empty after Close, got %d entries", got)
	}
}

// TestAlive_CloseBeforeRunClearsQueuedWork verifies that Close on a loop that
// never started still clears queued work so Alive() becomes false immediately.
func TestAlive_CloseBeforeRunClearsQueuedWork(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := loop.ScheduleTimer(time.Hour, func() {}); err != nil {
		t.Fatalf("ScheduleTimer before Run: %v", err)
	}
	if err := loop.ScheduleMicrotask(func() {}); err != nil {
		t.Fatalf("ScheduleMicrotask before Run: %v", err)
	}
	if err := loop.ScheduleNextTick(func() {}); err != nil {
		t.Fatalf("ScheduleNextTick before Run: %v", err)
	}
	if err := loop.Submit(func() {}); err != nil {
		t.Fatalf("Submit before Run: %v", err)
	}

	if !loop.Alive() {
		t.Fatal("Alive() should return true with queued pre-Run work")
	}

	if err := loop.Close(); err != nil {
		t.Fatalf("Close before Run: %v", err)
	}

	if loop.Alive() {
		t.Fatal("Alive() should return false after Close clears pre-Run queued work")
	}
	loop.internalQueueMu.Lock()
	internalLen := loop.internal.Length()
	loop.internalQueueMu.Unlock()
	if internalLen != 0 {
		t.Fatalf("internal queue should be empty after Close, got %d entries", internalLen)
	}
	loop.externalMu.Lock()
	externalLen := loop.external.Length()
	auxLen := len(loop.auxJobs)
	loop.externalMu.Unlock()
	if externalLen != 0 {
		t.Fatalf("external queue should be empty after Close, got %d entries", externalLen)
	}
	if auxLen != 0 {
		t.Fatalf("auxJobs should be empty after Close, got %d entries", auxLen)
	}
	if !loop.microtasks.IsEmpty() {
		t.Fatal("microtasks should be empty after Close")
	}
	if !loop.nextTickQueue.IsEmpty() {
		t.Fatal("nextTickQueue should be empty after Close")
	}
}

// TestClose_Idempotent_NoPanic verifies that calling Close() twice on the same
// Loop does not panic, double-free timers to the pool, or corrupt state.
// The second call should be a no-op that returns nil.
func TestClose_Idempotent_NoPanic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = loop.Run(ctx) }()
	waitForLoopRunning(t, loop, 2*time.Second)

	_, err = loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	// First Close
	if err := loop.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	if loop.Alive() {
		t.Fatal("Alive() should return false after first Close")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount should be 0 after first Close, got %d", got)
	}

	// Second Close — must not panic or corrupt state.
	// It may return ErrLoopTerminated since the loop is already terminated.
	if err := loop.Close(); err != nil && err != ErrLoopTerminated {
		t.Fatalf("second Close: %v", err)
	}

	if loop.Alive() {
		t.Fatal("Alive() should still return false after second Close")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount should still be 0 after second Close, got %d", got)
	}
	if got := len(loop.timers); got != 0 {
		t.Fatalf("timers heap should still be empty after second Close, got %d entries", got)
	}
}

// TestShutdown_ThenClose verifies that calling Shutdown() then Close() on the
// same Loop does not panic or corrupt state. This exercises the path where
// terminateCleanup is called from both shutdown and Close.
func TestShutdown_ThenClose(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = loop.Run(ctx) }()
	waitForLoopRunning(t, loop, 2*time.Second)

	_, err = loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	// Shutdown first
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if loop.Alive() {
		t.Fatal("Alive() should return false after Shutdown")
	}

	// Close after Shutdown — must not panic.
	// It may return ErrLoopTerminated since the loop is already terminated.
	if err := loop.Close(); err != nil && err != ErrLoopTerminated {
		t.Fatalf("Close after Shutdown: %v", err)
	}

	if loop.Alive() {
		t.Fatal("Alive() should return false after Close following Shutdown")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount should be 0, got %d", got)
	}
}

// TestClose_DuringPromisify_InFlight verifies that Close() correctly waits for
// in-flight Promisify goroutines and cleans up state after they complete.
func TestClose_DuringPromisify_InFlight(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = loop.Run(ctx) }()
	waitForLoopRunning(t, loop, 2*time.Second)

	// Start a Promisify goroutine that blocks until we signal it
	unblock := make(chan struct{})
	p := loop.Promisify(context.Background(), func(ctx context.Context) (any, error) {
		<-unblock
		return "done", nil
	})

	// Give the goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Close while the Promisify goroutine is still in-flight
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- loop.Close()
	}()

	// Give Close() time to reach the promisifyWg.Wait() barrier
	time.Sleep(50 * time.Millisecond)

	// Now unblock the Promisify goroutine
	close(unblock)

	// Close should complete after the goroutine finishes
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not complete within 5 seconds — deadlock?")
	}

	// Verify cleanup
	if loop.Alive() {
		t.Fatal("Alive() should return false after Close")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount should be 0 after Close, got %d", got)
	}

	// Verify the promise settled (either resolved or rejected due to termination)
	deadline := time.After(2 * time.Second)
	select {
	case <-p.ToChannel():
		// Promise settled — expected
	case <-deadline:
		t.Fatal("Promisify promise should have settled after Close")
	}
}

// TestShutdown_NextTickPriority_OverMicrotasks verifies that during shutdown,
// nextTick callbacks execute before microtask callbacks, matching the priority
// order in the normal drainMicrotasks path.
func TestShutdown_NextTickPriority_OverMicrotasks(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = loop.Run(ctx) }()
	waitForLoopRunning(t, loop, 2*time.Second)

	// Track execution order using a slice protected by a mutex.
	// We can't use atomic append since we need to append and read atomically.
	var mu sync.Mutex
	var orderLog []int32

	// Schedule both nextTick and microtask from within the loop so they
	// are queued during shutdown drain. Use a ScheduleNextTick callback
	// that also queues a microtask, creating a controlled ordering test.
	_ = loop.SubmitInternal(func() {
		// Queue nextTick first (should execute first during drain)
		_ = loop.ScheduleNextTick(func() {
			mu.Lock()
			orderLog = append(orderLog, 10)
			mu.Unlock()
		})
		// Queue microtask second (should execute after nextTick)
		_ = loop.ScheduleMicrotask(func() {
			mu.Lock()
			orderLog = append(orderLog, 20)
			mu.Unlock()
		})
	})

	// Give time for the SubmitInternal to execute
	time.Sleep(50 * time.Millisecond)

	// Shutdown triggers the drain loop
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// Verify both fired and nextTick came before microtask
	mu.Lock()
	logLen := len(orderLog)
	if logLen != 2 {
		mu.Unlock()
		t.Fatalf("expected 2 callbacks, got %d", logLen)
	}
	if orderLog[0] != 10 || orderLog[1] != 20 {
		mu.Unlock()
		t.Fatalf("expected order [10, 20], got %v", orderLog)
	}
	mu.Unlock()
}
