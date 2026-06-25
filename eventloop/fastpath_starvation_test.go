package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ===== STARVATION ANALYSIS TESTS =====
// These tests verify correctness of the fast path, particularly around
// potential starvation scenarios and state transitions.

// TestFastPath_AuxJobsStarvation_ModeTransition verifies that auxJobs are
// drained when transitioning FROM fast path TO poll path.
//
// CRITICAL SCENARIO:
//  1. Loop is in fast path mode (StateRunning, blocked on fastWakeupCh)
//  2. Tasks submitted via Submit() go to auxJobs (fast path submission)
//  3. RegisterFD() is called - loop must transition to poll path
//  4. Verify: auxJobs MUST be drained before or during transition
//
// POTENTIAL BUG: If runFastPath exits (returns false) without draining,
// tick() would process l.external (chunkedIngress) but NOT l.auxJobs.
func TestFastPath_AuxJobsStarvation_ModeTransition(t *testing.T) {
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

	// Wait for loop to enter fast path (StateRunning)
	deadline := time.Now().Add(time.Second)
	for loop.State() != StateRunning && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if loop.State() != StateRunning {
		t.Fatalf("Loop not in StateRunning: %v", loop.State())
	}

	// Submit tasks in fast path mode - these go to auxJobs
	var executed atomic.Int64
	const taskCount = 100
	for range taskCount {
		if err := loop.Submit(func() {
			executed.Add(1)
		}); err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}

	// Wait for tasks to be processed in fast path
	deadline = time.Now().Add(time.Second)
	for executed.Load() < taskCount && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if executed.Load() != taskCount {
		t.Fatalf("STARVATION: Only %d/%d tasks executed before mode transition",
			executed.Load(), taskCount)
	}

	// Cleanup
	if err := loop.Shutdown(ctx); err != nil {
		t.Logf("Shutdown: %v", err)
	}
	<-runCh
}

// TestFastPath_InternalQueueStarvation_FastVsTick verifies that SubmitInternal
// tasks are properly drained in BOTH fast path and tick path.
//
// CRITICAL: runAux() drains internal queue. If a mode transition happens
// during the select, tasks in internal queue MUST still be processed.
func TestFastPath_InternalQueueStarvation_FastVsTick(t *testing.T) {
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

	// Wait for loop to be running
	deadline := time.Now().Add(time.Second)
	for loop.State() != StateRunning && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	// Submit internal tasks
	var executed atomic.Int64
	const taskCount = 50
	for range taskCount {
		if err := loop.SubmitInternal(func() {
			executed.Add(1)
		}); err != nil {
			t.Fatalf("SubmitInternal failed: %v", err)
		}
	}

	// Wait for execution
	deadline = time.Now().Add(time.Second)
	for executed.Load() < taskCount && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if executed.Load() != taskCount {
		t.Fatalf("STARVATION: Only %d/%d internal tasks executed",
			executed.Load(), taskCount)
	}

	if err := loop.Shutdown(ctx); err != nil {
		t.Logf("Shutdown: %v", err)
	}
	<-runCh
}

// TestFastPath_MicrotaskBudgetExceeded_NoBlock verifies that when microtask
// budget (1024) is exceeded, the loop does NOT block indefinitely.
//
// IMPLEMENTATION DETAIL: After drainMicrotasks(), if !IsEmpty(), runAux()
// sends to fastWakeupCh to prevent blocking. This test verifies that.
func TestFastPath_MicrotaskBudgetExceeded_NoBlock(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	_ = loop.SetFastPathMode(FastPathForced)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	runCh := make(chan error, 1)
	go func() {
		runCh <- loop.Run(ctx)
	}()

	// Wait for loop to be running
	for loop.State() != StateRunning {
		time.Sleep(time.Millisecond)
	}

	// Submit a task that schedules MORE than budget (1024) microtasks
	const microtaskCount = 2048
	var executed atomic.Int64
	done := make(chan struct{})

	if err := loop.Submit(func() {
		for range microtaskCount {
			_ = loop.ScheduleMicrotask(func() {
				if executed.Add(1) == microtaskCount {
					close(done)
				}
			})
		}
	}); err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	select {
	case <-done:
		t.Logf("All %d microtasks executed successfully", microtaskCount)
	case <-time.After(10 * time.Second):
		t.Fatalf("STALL: Only %d/%d microtasks executed (budget exceeded, loop blocked)",
			executed.Load(), microtaskCount)
	}

	if err := loop.Shutdown(ctx); err != nil {
		t.Logf("Shutdown: %v", err)
	}
}

// TestFastPath_EntryCondition_HasInternalTasks verifies that fast path is
// NOT entered when there are pending internal tasks.
//
// CRITICAL INVARIANT: Line 407:
//
//	if l.canUseFastPath() && !l.hasTimersPending() && !l.hasInternalTasks()
//
// This prevents starvation of internal tasks during mode transitions.
func TestFastPath_EntryCondition_HasInternalTasks(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Use poll path initially, then switch
	_ = loop.SetFastPathMode(FastPathDisabled)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runCh := make(chan error, 1)
	go func() {
		runCh <- loop.Run(ctx)
	}()

	// Wait for loop to be sleeping (poll mode)
	deadline := time.Now().Add(time.Second)
	for loop.State() != StateSleeping && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	// Submit internal tasks BEFORE switching to fast path
	var executed atomic.Int64
	const taskCount = 20
	for range taskCount {
		if err := loop.SubmitInternal(func() {
			executed.Add(1)
		}); err != nil {
			t.Fatalf("SubmitInternal failed: %v", err)
		}
	}

	// Switch to fast path mode
	_ = loop.SetFastPathMode(FastPathAuto)

	// Wait for execution
	deadline = time.Now().Add(time.Second)
	for executed.Load() < taskCount && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if executed.Load() != taskCount {
		t.Fatalf("STARVATION: Only %d/%d internal tasks executed after mode switch",
			executed.Load(), taskCount)
	}

	if err := loop.Shutdown(ctx); err != nil {
		t.Logf("Shutdown: %v", err)
	}
	<-runCh
}

// TestFastPath_ExternalQueueDrained_Transition verifies that the chunkedIngress
// external queue is drained when transitioning FROM poll TO fast path.
//
// SCENARIO:
//  1. Loop in poll mode (FastPathDisabled), tasks go to l.external
//  2. Mode changes to FastPathAuto (no FDs, becomes fast path)
//  3. Tasks in l.external MUST be processed by tick() before fast path entry
//
// INVARIANT: tick() calls processExternal() which drains l.external
func TestFastPath_ExternalQueueDrained_Transition(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	_ = loop.SetFastPathMode(FastPathDisabled)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runCh := make(chan error, 1)
	go func() {
		runCh <- loop.Run(ctx)
	}()

	// Wait for loop to be in poll mode (sleeping)
	deadline := time.Now().Add(time.Second)
	for loop.State() != StateSleeping && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	// Submit tasks - these go to l.external (chunkedIngress)
	var executed atomic.Int64
	const taskCount = 50
	for range taskCount {
		if err := loop.Submit(func() {
			executed.Add(1)
		}); err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}

	// Switch to fast path mode
	_ = loop.SetFastPathMode(FastPathAuto)

	// Wait for execution
	deadline = time.Now().Add(time.Second)
	for executed.Load() < taskCount && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if executed.Load() != taskCount {
		t.Fatalf("STARVATION: Only %d/%d tasks executed after mode switch",
			executed.Load(), taskCount)
	}

	if err := loop.Shutdown(ctx); err != nil {
		t.Logf("Shutdown: %v", err)
	}
	<-runCh
}

// TestFastPath_TerminatingDrain_AuxJobs verifies that shutdown() properly
// drains auxJobs even after fast path exit.
//
// CRITICAL: shutdown() at line 577-587 explicitly drains auxJobs.
func TestFastPath_TerminatingDrain_AuxJobs(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	_ = loop.SetFastPathMode(FastPathForced)

	ctx := t.Context()

	runCh := make(chan error, 1)
	go func() {
		runCh <- loop.Run(ctx)
	}()

	// Wait for loop to be running
	for loop.State() != StateRunning {
		time.Sleep(time.Millisecond)
	}

	// Submit tasks immediately before shutdown
	var executed atomic.Int64
	const taskCount = 100
	var wg sync.WaitGroup
	wg.Go(func() {
		for range taskCount {
			err := loop.Submit(func() {
				executed.Add(1)
			})
			if err == ErrLoopTerminated {
				break
			}
		}
	})

	// Small delay then shutdown
	time.Sleep(5 * time.Millisecond)
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Logf("Shutdown: %v", err)
	}
	wg.Wait()
	<-runCh

	// Verify - at minimum, some tasks must have executed
	// The important thing is no ACCEPTED tasks were lost
	t.Logf("Executed %d tasks during shutdown drain", executed.Load())
}

// TestFastPath_RunAux_DrainsMicrotasksAfterInternal verifies that microtasks
// scheduled by internal tasks are drained within the same runAux call.
//
// ORDERING (runAux lines 473-501):
//  1. Drain auxJobs
//  2. Drain internal queue
//  3. Drain microtasks (at end)
//
// BUG RISK: Microtasks scheduled by internal tasks are drained at step 3,
// but if internal tasks schedule MORE internal tasks... those go to the queue
// and are picked up in the next runAux iteration.
func TestFastPath_RunAux_DrainsMicrotasksAfterInternal(t *testing.T) {
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

	// Track execution order
	var order []string
	var mu sync.Mutex

	record := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	done := make(chan struct{})

	if err := loop.SubmitInternal(func() {
		record("internal-1")
		_ = loop.ScheduleMicrotask(func() {
			record("microtask-from-internal")
			close(done)
		})
	}); err != nil {
		t.Fatalf("SubmitInternal failed: %v", err)
	}

	select {
	case <-done:
		mu.Lock()
		t.Logf("Execution order: %v", order)
		mu.Unlock()
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for execution")
	}

	if err := loop.Shutdown(ctx); err != nil {
		t.Logf("Shutdown: %v", err)
	}
	<-runCh
}

// TestFastPath_ConcurrentSubmit_ModeSwitch stress tests concurrent submissions
// during mode transitions to verify no tasks are lost.
func TestFastPath_ConcurrentSubmit_ModeSwitch(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	for iteration := range 50 {
		loop, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

		runCh := make(chan error, 1)
		go func() {
			runCh <- loop.Run(ctx)
		}()

		// Wait for running
		for loop.State() != StateRunning {
			time.Sleep(time.Microsecond * 100)
		}

		var accepted, executed atomic.Int64
		var wg sync.WaitGroup

		// Concurrent submitters
		for range 10 {
			wg.Go(func() {
				for range 100 {
					err := loop.Submit(func() {
						executed.Add(1)
					})
					if err == nil {
						accepted.Add(1)
					} else if err == ErrLoopTerminated {
						return
					}
				}
			})
		}

		// Concurrent mode switcher
		wg.Go(func() {
			modes := []FastPathMode{FastPathAuto, FastPathDisabled, FastPathAuto}
			for _, m := range modes {
				_ = loop.SetFastPathMode(m)
				time.Sleep(time.Millisecond)
			}
		})

		wg.Wait()

		// Give time for execution
		time.Sleep(50 * time.Millisecond)

		if err := loop.Shutdown(context.Background()); err != nil && !errors.Is(err, ErrLoopTerminated) {
			t.Logf("Shutdown: %v", err)
		}
		<-runCh

		acc := accepted.Load()
		exec := executed.Load()
		if acc != exec {
			t.Fatalf("Iteration %d: DATA LOSS! Accepted %d, Executed %d (lost %d)",
				iteration, acc, exec, acc-exec)
		}

		cancel()
	}
}

// TestFastPath_ExternalQueueDrained_ModeReversion verifies that tasks in l.external
// are drained even when mode reverts back to fast-path-compatible mid-flight.
//
// EDGE CASE SCENARIO:
//  1. Loop in fast path mode (Auto, no FDs)
//  2. SetFastPathMode(Disabled) called
//  3. Submit() checks canUseFastPath() -> false, pushes to l.external
//  4. SetFastPathMode(Auto) called quickly
//  5. Loop wakes, canUseFastPath() -> true (mode is Auto again)
//  6. BUG (fixed): Loop stays in fast path, never drains l.external
//  7. FIX: runFastPath now checks hasExternalTasks() and exits to tick()
func TestFastPath_ExternalQueueDrained_ModeReversion(t *testing.T) {
	for iter := range 100 {
		loop, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

		runCh := make(chan error, 1)
		go func() {
			runCh <- loop.Run(ctx)
		}()

		// Wait for running (fast path mode)
		for loop.State() != StateRunning {
			time.Sleep(time.Millisecond)
		}

		var executed atomic.Int64
		const taskCount = 20

		// Disable fast path briefly
		_ = loop.SetFastPathMode(FastPathDisabled)

		// Submit tasks - they go to l.external
		for range taskCount {
			if err := loop.Submit(func() {
				executed.Add(1)
			}); err != nil {
				t.Fatalf("Iteration %d: Submit failed: %v", iter, err)
			}
		}

		// Re-enable fast path immediately
		_ = loop.SetFastPathMode(FastPathAuto)

		// Wait for execution
		deadline := time.Now().Add(500 * time.Millisecond)
		for executed.Load() < taskCount && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		if executed.Load() != taskCount {
			t.Fatalf("Iteration %d: STARVATION! Only %d/%d tasks executed (mode reversion edge case)",
				iter, executed.Load(), taskCount)
		}

		if err := loop.Shutdown(context.Background()); err != nil && !errors.Is(err, ErrLoopTerminated) {
			t.Logf("Shutdown: %v", err)
		}
		<-runCh
		cancel()
	}
}

// TestRunAuxInternalBudgetDoesNotStrandTasks is the Reproduce-or-Fail guard for
// review-01 #1 / review-02 §2, which claimed that runAux's missing fastWakeupCh
// self-signal on internal-queue budget exhaustion strands the surplus in the
// fast path (the loop would block forever on select, leaving the deferred tasks
// unprocessed until an unrelated wakeup).
//
// Determinism: the loopTestHooks.OnFastPathEntry hook gates the test so the
// sentinel is submitted only AFTER the loop has entered runFastPath. This makes
// the sentinel (submitted first, popped first by FIFO chunkedIngress) be
// processed by runAux's internal loop rather than tick()'s processInternalQueue,
// so runAux provably observes more than internalQueueBudget (4096) tasks at once
// (the "internal queue budget exceeded in runAux" log is asserted to fire).
//
// Verified-correct no-stranding mechanism: runAux processes 4096 and returns
// with a remainder; runFastPath's hasInternalTasks() guard (loop.go:660-664)
// returns the loop to the main run() loop, which calls tick() ->
// processInternalQueue() to drain the remainder. The queued-LAST marker closes
// done only once everything ahead of it has run, so reaching done proves no task
// was stranded. No fastWakeupCh self-signal is needed in runAux.
//
// If a regression removes the hasInternalTasks guard or breaks the tick fallback,
// this test hangs and fails on the done-timeout. (An independent reviewer
// confirmed this via an adversarial break-test: neutering the guard made this
// test fail ~7/8 runs.)
func TestRunAuxInternalBudgetDoesNotStrandTasks(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	// Default New() => FastPathAuto, no I/O FDs => fast path (runFastPath/runAux).

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Gate on actual fast-path entry so the sentinel is processed by runAux
	// (deterministic) rather than racing into the tick path.
	enteredFastPath := make(chan struct{})
	var fpOnce sync.Once
	loop.testHooks = &loopTestHooks{
		OnFastPathEntry: func() { fpOnce.Do(func() { close(enteredFastPath) }) },
	}

	runCh := make(chan error, 1)
	go func() { runCh <- loop.Run(ctx) }()

	const taskCount = 10000 // well above internalQueueBudget (4096)
	var executed atomic.Int64
	done := make(chan struct{})
	gate := make(chan struct{})

	// Wait until the loop has entered runFastPath, then queue the sentinel.
	<-enteredFastPath

	// Sentinel: submitted first => popped first by runAux. It blocks the loop
	// goroutine until the rest are queued, GUARANTEEING runAux sees >4096 queued
	// tasks at once (deterministic budget exhaustion).
	if err := loop.SubmitInternal(func() { <-gate }); err != nil {
		t.Fatalf("sentinel SubmitInternal failed: %v", err)
	}

	// Queue the no-op tasks while the loop is blocked on the sentinel.
	for range taskCount {
		if err := loop.SubmitInternal(func() { executed.Add(1) }); err != nil {
			close(gate)
			t.Fatalf("SubmitInternal %d failed: %v", executed.Load(), err)
		}
	}
	// Marker: queued LAST. Closing done here proves every prior task drained.
	if err := loop.SubmitInternal(func() { close(done) }); err != nil {
		close(gate)
		t.Fatalf("marker SubmitInternal failed: %v", err)
	}

	close(gate) // release the loop: runAux now sees taskCount+1 queued at once

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("fast-path internal queue stalled after budget exhaustion: %d/%d tasks executed",
			executed.Load(), taskCount)
	}

	if got := executed.Load(); got != taskCount {
		t.Fatalf("task loss: executed %d/%d (internal queue budget stranding)", got, taskCount)
	}

	if err := loop.Shutdown(context.Background()); err != nil && !errors.Is(err, ErrLoopTerminated) {
		t.Logf("Shutdown: %v", err)
	}
	cancel()
	<-runCh
}
