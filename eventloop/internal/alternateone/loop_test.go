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
	_ = loop.Stop(ctx)
}

// TestStartStop verifies basic start/stop cycle.
func TestStartStop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	// Start
	ctx := context.Background()
	if err := loop.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	// Verify running
	time.Sleep(10 * time.Millisecond)
	state := loop.State()
	if state != StateRunning && state != StateSleeping {
		t.Errorf("State after Start = %v, want Running or Sleeping", state)
	}
	// Stop
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := loop.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
	// Verify terminated
	if loop.State() != StateTerminated {
		t.Errorf("State after Stop = %v, want StateTerminated", loop.State())
	}
}

// TestSubmit verifies task submission and execution.
func TestSubmit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	ctx := context.Background()
	if err := loop.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
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
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = loop.Stop(stopCtx)
}

// TestSubmitInternal verifies internal priority queue.
func TestSubmitInternal(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	ctx := context.Background()
	if err := loop.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
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
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = loop.Stop(stopCtx)
}

// TestScheduleTimer verifies timer scheduling.
func TestScheduleTimer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	ctx := context.Background()
	if err := loop.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
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
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = loop.Stop(stopCtx)
}

// TestMultipleSubmits verifies multiple concurrent submissions.
func TestMultipleSubmits(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	ctx := context.Background()
	if err := loop.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
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
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = loop.Stop(stopCtx)
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

// TestSubmitAfterStop verifies rejection after stop.
func TestSubmitAfterStop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	ctx := context.Background()
	if err := loop.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	// Stop
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := loop.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
	// Submit should fail
	err = loop.Submit(func() {})
	if err != ErrLoopTerminated {
		t.Errorf("Submit after Stop returned %v, want ErrLoopTerminated", err)
	}
}

// TestStopIdempotence verifies Stop() is idempotent.
func TestStopIdempotence(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	ctx := context.Background()
	if err := loop.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	// Multiple Stop calls
	var wg sync.WaitGroup
	const numStops = 10
	wg.Add(numStops)
	for i := 0; i < numStops; i++ {
		go func() {
			defer wg.Done()
			stopCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			_ = loop.Stop(stopCtx)
		}()
	}
	wg.Wait()
	// Should be terminated
	if loop.State() != StateTerminated {
		t.Errorf("State = %v, want StateTerminated", loop.State())
	}
}

// TestStopUnstartedLoop verifies Stop on unstarted loop.
func TestStopUnstartedLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	// Stop without Start
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := loop.Stop(ctx); err != nil {
		t.Fatalf("Stop() on unstarted loop failed: %v", err)
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
	ctx := context.Background()
	if err := loop.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
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
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = loop.Stop(stopCtx)
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
	ctx := context.Background()
	if err := loop.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = loop.Stop(stopCtx)
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
