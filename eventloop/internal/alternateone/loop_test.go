package alternateone

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournamenttest"
)

const alternateOneTestTimeout = 5 * time.Second

type alternateOneTestLoop struct {
	loop        *Loop
	runDone     chan error
	cleanupOnce sync.Once
}

func startAlternateOneTestLoop(t testing.TB, loop *Loop) *alternateOneTestLoop {
	t.Helper()
	loop.SetShutdownLogger(nil)
	harness := &alternateOneTestLoop{
		loop:    loop,
		runDone: make(chan error, 1),
	}
	go func() { harness.runDone <- loop.Run(context.Background()) }()
	t.Cleanup(func() { harness.cleanup(t) })

	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		t.Fatalf("Submit(start barrier) failed: %v", err)
	}
	waitAlternateOneSignal(t, ready, "start barrier")
	return harness
}

func (h *alternateOneTestLoop) cleanup(t testing.TB) {
	t.Helper()
	h.cleanupOnce.Do(func() {
		result := tournamenttest.Terminate(h.loop, h.runDone, alternateOneTestTimeout)
		if result.ShutdownErr != nil && !errors.Is(result.ShutdownErr, ErrLoopTerminated) {
			t.Errorf("cleanup Shutdown failed: %v", result.ShutdownErr)
		}
		if result.CloseErr != nil && !errors.Is(result.CloseErr, ErrLoopTerminated) {
			t.Errorf("cleanup Close failed: %v", result.CloseErr)
		}
		if result.RunErr != nil {
			t.Errorf("Run() failed: %v", result.RunErr)
		}
	})
}

func (h *alternateOneTestLoop) shutdown(t testing.TB) {
	t.Helper()
	h.cleanupOnce.Do(func() {
		result := tournamenttest.Terminate(h.loop, h.runDone, alternateOneTestTimeout)
		if result.ShutdownErr != nil {
			t.Fatalf("Shutdown() failed: %v", result.ShutdownErr)
		}
		if result.CloseErr != nil && !errors.Is(result.CloseErr, ErrLoopTerminated) {
			t.Fatalf("fallback Close() failed: %v", result.CloseErr)
		}
		if result.RunErr != nil {
			t.Fatalf("Run() failed: %v", result.RunErr)
		}
	})
}

func (h *alternateOneTestLoop) wait(t testing.TB) {
	t.Helper()
	timer := time.NewTimer(alternateOneTestTimeout)
	defer timer.Stop()
	select {
	case err := <-h.runDone:
		h.runDone <- err
		if err != nil {
			t.Errorf("Run() failed: %v", err)
		}
	case <-timer.C:
		t.Fatalf("Run() did not return within %v", alternateOneTestTimeout)
	}
}

func waitAlternateOneSignal(t testing.TB, signal <-chan struct{}, description string) {
	t.Helper()
	timer := time.NewTimer(alternateOneTestTimeout)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", description)
	}
}

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
	ctx, cancel := context.WithTimeout(context.Background(), alternateOneTestTimeout)
	defer cancel()
	if err := loop.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() failed: %v", err)
	}
}

// TestRunShutdown verifies basic run/shutdown cycle.
func TestRunShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateOneTestLoop(t, loop)

	state := loop.State()
	if state != StateRunning && state != StateSleeping {
		t.Errorf("State after Run = %v, want Running or Sleeping", state)
	}

	harness.shutdown(t)

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

	harness := startAlternateOneTestLoop(t, loop)

	var executed atomic.Bool
	done := make(chan struct{})
	if err := loop.Submit(func() {
		executed.Store(true)
		close(done)
	}); err != nil {
		t.Fatalf("Submit() failed: %v", err)
	}

	// Wait for execution
	waitAlternateOneSignal(t, done, "submitted task")

	if !executed.Load() {
		t.Error("Task was not executed")
	}

	harness.shutdown(t)
}

// TestSubmitInternal verifies internal priority queue.
func TestSubmitInternal(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateOneTestLoop(t, loop)

	var executed atomic.Bool
	done := make(chan struct{})
	if err := loop.SubmitInternal(func() {
		executed.Store(true)
		close(done)
	}); err != nil {
		t.Fatalf("SubmitInternal() failed: %v", err)
	}

	// Wait for execution
	waitAlternateOneSignal(t, done, "internal task")

	if !executed.Load() {
		t.Error("Internal task was not executed")
	}

	harness.shutdown(t)
}

// TestScheduleTimer verifies timer scheduling.
func TestScheduleTimer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateOneTestLoop(t, loop)

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
	waitAlternateOneSignal(t, done, "scheduled timer")
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Errorf("Timer executed too early: %v", elapsed)
	}

	if !executed.Load() {
		t.Error("Timer was not executed")
	}

	harness.shutdown(t)
}

// TestMultipleSubmits verifies multiple concurrent submissions.
func TestMultipleSubmits(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateOneTestLoop(t, loop)

	const numTasks = 100
	var counter atomic.Int32
	for range numTasks {
		if err := loop.Submit(func() {
			counter.Add(1)
		}); err != nil {
			t.Fatalf("Submit() failed: %v", err)
		}
	}
	done := make(chan struct{})
	if err := loop.Submit(func() { close(done) }); err != nil {
		t.Fatalf("Submit(drain barrier) failed: %v", err)
	}
	waitAlternateOneSignal(t, done, "submitted task drain")

	if counter.Load() != numTasks {
		t.Errorf("Executed %d tasks, want %d", counter.Load(), numTasks)
	}

	harness.shutdown(t)
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

	harness := startAlternateOneTestLoop(t, loop)
	harness.shutdown(t)

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

	harness := startAlternateOneTestLoop(t, loop)

	// Multiple Shutdown calls
	const numShutdowns = 10
	results := make(chan error, numShutdowns)
	start := make(chan struct{})
	for range numShutdowns {
		go func() {
			<-start
			shutdownCtx, cancelCtx := context.WithTimeout(context.Background(), alternateOneTestTimeout)
			defer cancelCtx()
			results <- loop.Shutdown(shutdownCtx)
		}()
	}
	close(start)
	for range numShutdowns {
		select {
		case err := <-results:
			if err != nil {
				t.Errorf("concurrent Shutdown() returned %v, want nil", err)
			}
		case <-time.After(alternateOneTestTimeout):
			t.Fatal("concurrent Shutdown() calls did not all return")
		}
	}
	harness.wait(t)

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

	harness := startAlternateOneTestLoop(t, loop)

	var executedAfterPanic atomic.Bool
	done := make(chan struct{})

	// Submit panicking task
	if err := loop.Submit(func() {
		panic("test panic")
	}); err != nil {
		t.Fatalf("Submit(panic) failed: %v", err)
	}

	// Submit task after panic
	if err := loop.Submit(func() {
		executedAfterPanic.Store(true)
		close(done)
	}); err != nil {
		t.Fatalf("Submit(post-panic) failed: %v", err)
	}

	// Wait for execution
	waitAlternateOneSignal(t, done, "post-panic task")

	if !executedAfterPanic.Load() {
		t.Error("Task after panic was not executed")
	}

	harness.shutdown(t)
}

// TestSafeIngressInvariants verifies queue invariants.
func TestSafeIngressInvariants(t *testing.T) {
	q := NewSafeIngress()
	// Push and pop
	for range 1000 {
		if err := q.Push(func() {}, LaneExternal); err != nil {
			t.Fatalf("Push() failed: %v", err)
		}
	}
	for i := range 1000 {
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
	for i := range 50 {
		c.tasks[i] = SafeTask{ID: uint64(i + 1)}
	}
	c.pos = 50
	// Return chunk (should clear ALL 128 slots)
	returnChunk(c)
	// Get it back from pool
	c2 := newChunk()
	// Verify all slots are cleared
	for i := range chunkSize {
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

	harness := startAlternateOneTestLoop(t, loop)
	harness.shutdown(t)
	mu.Lock()
	defer mu.Unlock()
	if len(transitions) == 0 {
		t.Fatal("No state transitions observed")
	}
	// First transition should be Awake -> Running
	if len(transitions) > 0 && (transitions[0].from != StateAwake || transitions[0].to != StateRunning) {
		t.Errorf("First transition = %v -> %v, want StateAwake -> StateRunning", transitions[0].from, transitions[0].to)
	}
	last := transitions[len(transitions)-1]
	if last.from != StateTerminating || last.to != StateTerminated {
		t.Errorf("Last transition = %v -> %v, want StateTerminating -> StateTerminated", last.from, last.to)
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

	harness := startAlternateOneTestLoop(t, loop)

	// Hold an already-entered callback. Immediate Close must publish termination
	// without waiting for the callback to return.
	var executed atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseCallback)
	if err := loop.Submit(func() {
		executed.Store(true)
		close(entered)
		<-release
	}); err != nil {
		t.Fatalf("Submit() failed: %v", err)
	}
	waitAlternateOneSignal(t, entered, "callback entry before Close")

	closeResult := make(chan error, 1)
	go func() { closeResult <- loop.Close() }()
	select {
	case err := <-closeResult:
		if err != nil {
			t.Fatalf("Close() failed: %v", err)
		}
	case <-time.After(alternateOneTestTimeout):
		t.Fatal("Close() waited for an already-entered callback")
	}
	releaseCallback()

	if !executed.Load() {
		t.Error("held callback did not enter")
	}
	harness.wait(t)

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

	harness := startAlternateOneTestLoop(t, loop)

	// Run should still be blocking
	select {
	case err := <-harness.runDone:
		harness.runDone <- err
		t.Fatalf("Run() returned early with error: %v", err)
	default:
		// OK - still blocking
	}

	// Shutdown to unblock
	harness.shutdown(t)
}

// TestCloseAfterShutdown verifies the historical terminal follow-up result.
func TestCloseAfterShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateOneTestLoop(t, loop)
	harness.shutdown(t)

	// Should be terminated
	if loop.State() != StateTerminated {
		t.Errorf("State = %v, want StateTerminated", loop.State())
	}

	// Should be idempotent
	if err := loop.Close(); err != ErrLoopTerminated {
		t.Errorf("Close after Shutdown returned %v, want ErrLoopTerminated", err)
	}
}
