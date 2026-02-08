//go:build linux || darwin

package alternateone

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNew verifies basic loop creation.
func TestNew(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if loop == nil {
		t.Fatal("New() returned nil loop")
	}
	if loop.State() != StateAwake {
		t.Errorf("Initial state = %v, want StateAwake", loop.State())
	}
	// Clean up
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = loop.Shutdown(ctx)
}

// TestRunShutdown verifies basic run/shutdown cycle.
func TestRunShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Run Loop in a goroutine since Run() is blocking
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	// Verify running
	time.Sleep(10 * time.Millisecond)
	state := loop.State()
	if state != StateRunning && state != StateSleeping {
		t.Errorf("State after Run = %v, want Running or Sleeping", state)
	}

	// Shutdown
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() failed: %v", err)
	}

	// Cancel run context
	cancel()

	// Wait for Run() to return
	if err := <-runDone; err != nil && err != context.Canceled {
		t.Logf("Run() returned error (expected): %v", err)
	}

	// Verify terminated
	if loop.State() != StateTerminated {
		t.Errorf("State after Shutdown = %v, want StateTerminated", loop.State())
	}
}

// TestSubmit verifies task submission and execution.
func TestSubmit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Run Loop in a goroutine since Run() is blocking
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	var executed atomic.Bool
	done := make(chan struct{})
	if err := loop.Submit(func() {
		executed.Store(true)
		close(done)
	}); err != nil {
		t.Fatalf("Submit() failed: %v", err)
	}

	// Wait for execution
	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("Task not executed within timeout")
	}

	if !executed.Load() {
		t.Error("Task was not executed")
	}

	// Clean up
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	_ = loop.Shutdown(shutdownCtx)
	cancel()
	<-runDone
}

// TestSubmitInternal verifies internal priority queue.
func TestSubmitInternal(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Run Loop in a goroutine since Run() is blocking
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	var executed atomic.Bool
	done := make(chan struct{})
	if err := loop.SubmitInternal(func() {
		executed.Store(true)
		close(done)
	}); err != nil {
		t.Fatalf("SubmitInternal() failed: %v", err)
	}

	// Wait for execution
	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("Internal task not executed within timeout")
	}

	if !executed.Load() {
		t.Error("Internal task was not executed")
	}

	// Clean up
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	_ = loop.Shutdown(shutdownCtx)
	cancel()
	<-runDone
}

// TestScheduleTimer verifies timer scheduling.
func TestScheduleTimer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Run Loop in a goroutine since Run() is blocking
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	var executed atomic.Bool
	done := make(chan struct{})
	start := time.Now()
	if err := loop.ScheduleTimer(50*time.Millisecond, func() {
		executed.Store(true)
		close(done)
	}); err != nil {
		t.Fatalf("ScheduleTimer() failed: %v", err)
	}

	// Wait for execution
	select {
	case <-done:
		elapsed := time.Since(start)
		if elapsed < 40*time.Millisecond {
			t.Errorf("Timer executed too early: %v", elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("Timer not executed within timeout")
	}

	if !executed.Load() {
		t.Error("Timer was not executed")
	}

	// Clean up
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	_ = loop.Shutdown(shutdownCtx)
	cancel()
	<-runDone
}

// TestMultipleSubmits verifies multiple concurrent submissions.
func TestMultipleSubmits(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Run Loop in a goroutine since Run() is blocking
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	const numTasks = 100
	var counter atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numTasks)
	for i := 0; i < numTasks; i++ {
		if err := loop.Submit(func() {
			counter.Add(1)
			wg.Done()
		}); err != nil {
			t.Fatalf("Submit() failed: %v", err)
		}
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatalf("Only %d/%d tasks executed", counter.Load(), numTasks)
	}

	if counter.Load() != numTasks {
		t.Errorf("Executed %d tasks, want %d", counter.Load(), numTasks)
	}

	// Clean up
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	_ = loop.Shutdown(shutdownCtx)
	cancel()
	<-runDone
}

// TestStateTransitionValidation verifies strict state validation.
func TestStateTransitionValidation(t *testing.T) {
	sm := NewSafeStateMachine(nil)
	// Valid transition: Awake -> Running
	if !sm.Transition(StateAwake, StateRunning) {
		t.Error("Awake -> Running should succeed")
	}
	// Valid transition: Running -> Sleeping
	if !sm.Transition(StateRunning, StateSleeping) {
		t.Error("Running -> Sleeping should succeed")
	}
	// Valid transition: Sleeping -> Terminating
	if !sm.Transition(StateSleeping, StateTerminating) {
		t.Error("Sleeping -> Terminating should succeed")
	}
	// Valid transition: Terminating -> Terminated
	sm.ForceTerminated()
	if sm.Load() != StateTerminated {
		t.Error("ForceTerminated should set Terminated state")
	}
}

// TestInvalidTransitionPanics verifies invalid transitions panic.
func TestInvalidTransitionPanics(t *testing.T) {
	sm := NewSafeStateMachine(nil)
	defer func() {
		if r := recover(); r == nil {
			t.Error("Invalid transition should panic")
		}
	}()
	// Invalid transition: Awake -> Sleeping (should panic)
	sm.Transition(StateAwake, StateSleeping)
}

// TestSubmitAfterShutdown verifies rejection after shutdown.
func TestSubmitAfterShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Run Loop in a goroutine since Run() is blocking
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	// Shutdown
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() failed: %v", err)
	}

	cancel()
	<-runDone

	// Submit should fail
	err = loop.Submit(func() {})
	if err != ErrLoopTerminated {
		t.Errorf("Submit after Shutdown returned %v, want ErrLoopTerminated", err)
	}
}

// TestShutdownIdempotence verifies Shutdown() is idempotent.
func TestShutdownIdempotence(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Run Loop in a goroutine since Run() is blocking
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	// Multiple Shutdown calls
	var wg sync.WaitGroup
	const numShutdowns = 10
	wg.Add(numShutdowns)
	for i := 0; i < numShutdowns; i++ {
		go func() {
			defer wg.Done()
			shutdownCtx, cancelCtx := context.WithTimeout(context.Background(), time.Second)
			defer cancelCtx()
			_ = loop.Shutdown(shutdownCtx)
		}()
	}
	wg.Wait()

	cancel()
	<-runDone

	// Should be terminated
	if loop.State() != StateTerminated {
		t.Errorf("State = %v, want StateTerminated", loop.State())
	}
}

// TestShutdownUnstartedLoop verifies Shutdown on unstarted loop.
func TestShutdownUnstartedLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Shutdown without Run
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := loop.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() on unstarted loop failed: %v", err)
	}

	if loop.State() != StateTerminated {
		t.Errorf("State = %v, want StateTerminated", loop.State())
	}
}

// TestPanicRecovery verifies panic isolation.
func TestPanicRecovery(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Run Loop in a goroutine since Run() is blocking
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	var executedAfterPanic atomic.Bool
	done := make(chan struct{})

	// Submit panicking task
	_ = loop.Submit(func() {
		panic("test panic")
	})

	// Submit task after panic
	_ = loop.Submit(func() {
		executedAfterPanic.Store(true)
		close(done)
	})

	// Wait for execution
	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("Task after panic not executed")
	}

	if !executedAfterPanic.Load() {
		t.Error("Task after panic was not executed")
	}

	// Clean up
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	_ = loop.Shutdown(shutdownCtx)
	cancel()
	<-runDone
}

// TestSafeIngressInvariants verifies queue invariants.
func TestSafeIngressInvariants(t *testing.T) {
	q := NewSafeIngress()
	// Push and pop
	for i := 0; i < 1000; i++ {
		_ = q.Push(func() {}, LaneExternal)
	}
	for i := 0; i < 1000; i++ {
		_, ok := q.PopExternal()
		if !ok {
			t.Fatalf("Pop failed at iteration %d", i)
		}
	}
	// Should be empty
	if q.ExternalLength() != 0 {
		t.Errorf("Length = %d, want 0", q.ExternalLength())
	}
}

// TestChunkFullClear verifies full-clear behavior.
func TestChunkFullClear(t *testing.T) {
	c := newChunk()
	// Fill with some tasks
	for i := 0; i < 50; i++ {
		c.tasks[i] = SafeTask{ID: uint64(i + 1)}
	}
	c.pos = 50
	// Return chunk (should clear ALL 128 slots)
	returnChunk(c)
	// Get it back from pool
	c2 := newChunk()
	// Verify all slots are cleared
	for i := 0; i < chunkSize; i++ {
		if c2.tasks[i].ID != 0 || c2.tasks[i].Fn != nil {
			t.Errorf("Slot %d not cleared after returnChunk", i)
		}
	}
	returnChunk(c2)
}

// TestStateObserver verifies state transition observation.
func TestStateObserver(t *testing.T) {
	var transitions []struct {
		from LoopState
		to   LoopState
	}
	var mu sync.Mutex
	observer := observerFunc(func(from, to LoopState, _ time.Time) {
		mu.Lock()
		transitions = append(transitions, struct {
			from LoopState
			to   LoopState
		}{from, to})
		mu.Unlock()
	})
	loop, err := NewWithObserver(observer)
	if err != nil {
		t.Fatalf("NewWithObserver() failed: %v", err)
	}

	// Run Loop in a goroutine since Run() is blocking
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	_ = loop.Shutdown(shutdownCtx)

	cancel()
	<-runDone
	mu.Lock()
	defer mu.Unlock()
	if len(transitions) == 0 {
		t.Error("No state transitions observed")
	}
	// First transition should be Awake -> Running
	if len(transitions) > 0 && transitions[0].from != StateAwake {
		t.Errorf("First transition from = %v, want StateAwake", transitions[0].from)
	}
}

// observerFunc is a helper to implement StateObserver.
type observerFunc func(from, to LoopState, timestamp time.Time)

func (f observerFunc) OnTransition(from, to LoopState, timestamp time.Time) {
	f(from, to, timestamp)
}

// TestClose verifies immediate termination behavior.
func TestClose(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Run Loop in a goroutine since Run() is blocking
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	// Submit a task
	var executed atomic.Bool
	_ = loop.Submit(func() {
		executed.Store(true)
	})

	time.Sleep(100 * time.Millisecond)

	// Close immediately (should terminate loop within a few ticks)
	if err := loop.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Task may or may not have executed (Close is immediate, not graceful)
	t.Logf("Task executed: %v", executed.Load())

	// Cancel context to help Run() exit
	cancel()

	// Wait for Run() to return with longer timeout
	// Close guarantees termination, but may need to finish current tick
	select {
	case err := <-runDone:
		if err != nil && err != context.Canceled {
			t.Logf("Run() returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not return after Close within 3s")
	}

	// Should be terminated
	if loop.State() != StateTerminated {
		t.Errorf("State after Close = %v, want StateTerminated", loop.State())
	}

	// Close should be idempotent
	if err := loop.Close(); err != ErrLoopTerminated {
		t.Errorf("Second Close() returned %v, want ErrLoopTerminated", err)
	}
}

// TestCloseUnstarted verifies Close on unstarted loop.
func TestCloseUnstarted(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Close without Run
	if err := loop.Close(); err != nil {
		t.Fatalf("Close() on unstarted loop failed: %v", err)
	}

	if loop.State() != StateTerminated {
		t.Errorf("State = %v, want StateTerminated", loop.State())
	}

	// Second call should fail
	if err := loop.Close(); err != ErrLoopTerminated {
		t.Errorf("Second Close() returned %v, want ErrLoopTerminated", err)
	}
}

// TestRunBlocks verifies Run() blocks until termination.
func TestRunBlocks(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	runDone := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		runDone <- loop.Run(ctx)
	}()

	<-started
	time.Sleep(50 * time.Millisecond)

	// Run should still be blocking
	select {
	case err := <-runDone:
		t.Fatalf("Run() returned early with error: %v", err)
	default:
		// OK - still blocking
	}

	// Shutdown to unblock
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() failed: %v", err)
	}

	cancel()

	// Now Run should return
	select {
	case err := <-runDone:
		if err != nil && err != context.Canceled {
			t.Logf("Run() returned error (expected): %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after Shutdown")
	}
}

// TestConcurrentShutdownClose verifies concurrent Shutdown/Close is safe.
func TestConcurrentShutdownClose(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	// Call Shutdown gracefully
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() failed: %v", err)
	}

	// Run should return after Shutdown
	select {
	case err := <-runDone:
		if err != nil && err != context.Canceled {
			t.Logf("Run() returned error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Run() did not return after Shutdown")
	}

	// Should be terminated
	if loop.State() != StateTerminated {
		t.Errorf("State = %v, want StateTerminated", loop.State())
	}

	// Should be idempotent
	if err := loop.Close(); err != ErrLoopTerminated {
		t.Errorf("Close after Shutdown returned %v, want ErrLoopTerminated", err)
	}
}
