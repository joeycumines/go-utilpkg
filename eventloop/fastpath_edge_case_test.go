package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// ===== EDGE CASE ANALYSIS TESTS =====
// These tests probe specific edge cases in the fast path implementation.

// TestSubmit_RaceCondition_FastModeCheckBeforeLock probes a potential race
// in Submit() where fastMode is checked BEFORE acquiring the lock.
//
// CODE UNDER TEST (loop.go lines 922-949):
//
//	func (l *Loop) Submit(task func()) error {
//	    fastMode := l.canUseFastPath()  // STEP 1: Check mode (no lock)
//	    l.externalMu.Lock()              // STEP 2: Acquire lock
//	    ...
//	    if fastMode {                    // STEP 3: Use cached fastMode
//	        l.auxJobs = append(...)      // Task goes to auxJobs
//	    } else {
//	        l.external.Push(...)         // Task goes to external
//	    }
//	}
//
// RACE SCENARIO:
//
//	T1: Submit() checks fastMode=true (fast path)
//	T2: RegisterFD() increments userIOFDCount (now poll path)
//	T1: Submit() acquires lock, appends to auxJobs
//	RESULT: Task in auxJobs, but loop now in poll path
//
// QUESTION: Is this safe? YES, because:
//  1. RegisterFD() wakes the loop via fastWakeupCh AND submitWakeup()
//  2. runFastPath() will see !canUseFastPath() and return false
//  3. Main loop falls through to tick(), which processes auxJobs... NO WAIT
//
// CRITICAL INSIGHT: tick() does NOT process auxJobs!
//   - tick() calls processExternal() which drains l.external (ChunkedIngress)
//   - auxJobs are ONLY drained by runAux() in fast path mode
//
// POTENTIAL BUG: If Submit() puts task in auxJobs but loop transitions to
// poll path, the task is STARVED until:
//
//	a) Mode switches back to fast path, OR
//	b) Shutdown drains auxJobs (line 577-587)
//
// This test verifies the behavior.
func TestSubmit_RaceCondition_FastModeCheckBeforeLock(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runCh := make(chan error, 1)
	go func() {
		runCh <- loop.Run(ctx)
	}()

	// Wait for loop to be in fast path (StateRunning)
	for loop.State() != StateRunning {
		time.Sleep(time.Millisecond)
	}

	// Create pipe for FD registration
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	var executed atomic.Int64
	submitBarrier := make(chan struct{})
	registerDone := make(chan struct{})

	// Goroutine 1: Submit task (will check fastMode before lock)
	go func() {
		// Wait for signal to submit
		<-submitBarrier
		// This Submit() may check fastMode=true, then RegisterFD runs,
		// then Submit() appends to auxJobs despite loop being in poll mode now
		for i := 0; i < 10; i++ {
			if err := loop.Submit(func() {
				executed.Add(1)
			}); err != nil {
				t.Errorf("Submit failed: %v", err)
			}
		}
	}()

	// Goroutine 2: Register FD to force poll mode
	go func() {
		// Signal Submit to proceed, then immediately register FD
		close(submitBarrier)
		time.Sleep(time.Microsecond * 10) // Small delay to let Submit start
		err := loop.RegisterFD(fds[0], EventRead, func(IOEvents) {})
		if err != nil {
			t.Errorf("RegisterFD failed: %v", err)
		}
		close(registerDone)
	}()

	<-registerDone

	// Wait for tasks - if there's a starvation bug, tasks may not execute
	deadline := time.Now().Add(500 * time.Millisecond)
	for executed.Load() < 10 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	execCount := executed.Load()
	if execCount < 10 {
		// Check if tasks are stuck in auxJobs
		loop.externalMu.Lock()
		auxLen := len(loop.auxJobs)
		loop.externalMu.Unlock()

		if auxLen > 0 {
			t.Fatalf("STARVATION CONFIRMED: %d tasks stuck in auxJobs, only %d/10 executed. "+
				"Tasks submitted in fast path mode are not processed in poll path!",
				auxLen, execCount)
		}
		t.Fatalf("Missing tasks: only %d/10 executed", execCount)
	}

	// Cleanup
	loop.UnregisterFD(fds[0])
	if err := loop.Shutdown(ctx); err != nil && !errors.Is(err, ErrLoopTerminated) {
		t.Logf("Shutdown: %v", err)
	}
	<-runCh
}

// TestPollPath_DrainsBothQueues verifies that tick() drains BOTH auxJobs
// and l.external when in poll mode.
//
// CRITICAL: This test checks whether there's a starvation bug where
// auxJobs are not drained in poll path.
func TestPollPath_DrainsBothQueues(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Force poll path from the start
	_ = loop.SetFastPathMode(FastPathDisabled)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runCh := make(chan error, 1)
	go func() {
		runCh <- loop.Run(ctx)
	}()

	// Wait for poll mode (StateSleeping)
	deadline := time.Now().Add(time.Second)
	for loop.State() != StateSleeping && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	// Manually inject into auxJobs to simulate race condition
	loop.externalMu.Lock()
	var auxExecuted atomic.Int64
	for i := 0; i < 5; i++ {
		loop.auxJobs = append(loop.auxJobs, func() {
			auxExecuted.Add(1)
		})
	}
	loop.externalMu.Unlock()

	// Submit via normal path (goes to l.external)
	var externalExecuted atomic.Int64
	for i := 0; i < 5; i++ {
		if err := loop.Submit(func() {
			externalExecuted.Add(1)
		}); err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}

	// Wait for execution
	deadline = time.Now().Add(500 * time.Millisecond)
	for (auxExecuted.Load() < 5 || externalExecuted.Load() < 5) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	// Check results
	auxCount := auxExecuted.Load()
	extCount := externalExecuted.Load()

	if extCount != 5 {
		t.Errorf("l.external tasks: expected 5, got %d", extCount)
	}

	if auxCount != 5 {
		// CRITICAL: If this fails, auxJobs are NOT processed in poll path!
		t.Errorf("POTENTIAL BUG: auxJobs tasks: expected 5, got %d. "+
			"Tasks in auxJobs may be starved in poll mode!", auxCount)
	}

	if err := loop.Shutdown(ctx); err != nil {
		t.Logf("Shutdown: %v", err)
	}
	<-runCh
}

// TestFastPath_SubmitInternal_DirectExecution_StateRace probes SubmitInternal's
// direct execution path for race conditions.
//
// CODE (lines 998-1016):
//
//	if l.canUseFastPath() && state == StateRunning && l.isLoopThread() {
//	    l.externalMu.Lock()
//	    extLen := l.external.Length()
//	    l.externalMu.Unlock()
//	    if extLen == 0 {
//	        if l.state.Load() == StateTerminated {  // Re-check
//	            return ErrLoopTerminated
//	        }
//	        l.safeExecute(task)  // DIRECT EXECUTION
//	    }
//	}
//
// RACE: Between first state check and re-check, state could change.
// QUESTION: Is the re-check sufficient? It only checks StateTerminated,
// not StateTerminating.
func TestFastPath_SubmitInternal_DirectExecution_StateRace(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	runCh := make(chan error, 1)
	go func() {
		runCh <- loop.Run(ctx)
	}()

	// Wait for running
	for loop.State() != StateRunning {
		time.Sleep(time.Millisecond)
	}

	// Submit a task that calls SubmitInternal during termination
	var executed atomic.Int64
	var wg sync.WaitGroup

	// Start shutdown in parallel
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(time.Millisecond)
		_ = loop.Shutdown(context.Background())
	}()

	// Hammer SubmitInternal concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := loop.SubmitInternal(func() {
				executed.Add(1)
			})
			if err != nil && err != ErrLoopTerminated {
				t.Errorf("SubmitInternal unexpected error: %v", err)
			}
		}()
	}

	wg.Wait()
	<-runCh

	// No crash = success, log execution count
	t.Logf("Executed %d tasks during shutdown race", executed.Load())
}

// TestFastPath_TimerCreation_ExitsFastPath verifies that when a timer is
// scheduled, the fast path correctly exits.
//
// INVARIANT (line 407):
//
//	if l.canUseFastPath() && !l.hasTimersPending() && !l.hasInternalTasks()
//
// When timer is created, it goes via SubmitInternal which pushes to internal queue.
// Next loop iteration will see hasInternalTasks() = true and skip fast path.
// Then tick() will properly handle the timer.
func TestFastPath_TimerCreation_ExitsFastPath(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	_ = loop.SetFastPathMode(FastPathForced)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runCh := make(chan error, 1)
	go func() {
		runCh <- loop.Run(ctx)
	}()

	for loop.State() != StateRunning {
		time.Sleep(time.Millisecond)
	}

	// Schedule a timer from fast path
	fired := make(chan struct{})
	if err := loop.ScheduleTimer(10*time.Millisecond, func() {
		close(fired)
	}); err != nil {
		t.Fatalf("ScheduleTimer failed: %v", err)
	}

	select {
	case <-fired:
		t.Log("Timer fired successfully from fast path")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("TIMER NOT FIRED: Fast path may not be exiting for timer execution")
	}

	if err := loop.Shutdown(ctx); err != nil {
		t.Logf("Shutdown: %v", err)
	}
	<-runCh
}
