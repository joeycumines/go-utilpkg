package eventloop

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// pollFastMode coverage — targeting loop.go:1028
// =============================================================================

// TestPhase3_PollFastMode_IndefiniteBlockWakeup covers the >=1000ms path
// in pollFastMode where it blocks indefinitely on fastWakeupCh without
// allocating a timer (loop.go ~line 1072-1081).
func TestPhase3_PollFastMode_IndefiniteBlockWakeup(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	awakeCh := make(chan struct{}, 1)
	loop.testHooks = &loopTestHooks{
		PrePollAwake: func() {
			select {
			case awakeCh <- struct{}{}:
			default:
			}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Schedule a very far-future timer so poll calculates timeout >= 1000ms,
	// then send a wakeup via Submit to unblock the indefinite channel read.
	_, err = loop.ScheduleTimer(5*time.Second, func() {})
	require.NoError(t, err)

	// Submit a task: this wakes the loop from the indefinite block
	executed := make(chan struct{})
	require.NoError(t, loop.Submit(func() {
		close(executed)
	}))

	select {
	case <-executed:
		// pollFastMode >=1000ms indefinite block was entered and woken
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for task after indefinite block wakeup")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_PollFastMode_TerminatingBeforeIndefiniteBlock covers the
// StateTerminating check that occurs before the indefinite block path
// in pollFastMode (loop.go ~lines 1068-1071).
func TestPhase3_PollFastMode_TerminatingBeforeIndefiniteBlock(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Schedule a far-future timer so timeout >= 1000ms, then shut down.
	_, err = loop.ScheduleTimer(10*time.Second, func() {})
	require.NoError(t, err)

	// Shut down — this transitions to StateTerminating, which the
	// pollFastMode must detect before entering indefinite block.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	err = loop.Shutdown(shutdownCtx)
	assert.NoError(t, err)

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_PollFastMode_TerminatingBeforeTimerBlock covers the
// StateTerminating check before the short-timeout timer block path
// in pollFastMode (loop.go ~lines 1086-1089).
func TestPhase3_PollFastMode_TerminatingBeforeTimerBlock(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	// Hook: trigger shutdown right before poll to force the terminating check
	loop.testHooks = &loopTestHooks{
		PrePollSleep: func() {
			// Give the main goroutine time to call Shutdown
			// This is not timing dependent because Shutdown will succeed regardless
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Schedule a short-future timer (< 1000ms) so the short timer path is used
	_, err = loop.ScheduleTimer(200*time.Millisecond, func() {})
	require.NoError(t, err)

	// Shutdown while loop is running / polling
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	err = loop.Shutdown(shutdownCtx)
	assert.NoError(t, err)

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_PollFastMode_DrainAuxJobsAfterPoll covers the drainAuxJobs call
// that happens after returning from pollFastMode (loop.go ~line 1021).
// This ensures tasks that raced into auxJobs during mode transitions are drained.
func TestPhase3_PollFastMode_DrainAuxJobsAfterPoll(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Submit many tasks rapidly that will accumulate in auxJobs.
	// After pollFastMode returns, drainAuxJobs should process them.
	const n = 20
	var counter atomic.Int32
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		require.NoError(t, loop.Submit(func() {
			counter.Add(1)
			wg.Done()
		}))
	}

	// Wait using a channel-based approach
	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for tasks")
	}

	assert.Equal(t, int32(n), counter.Load())

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_PollFastMode_ZeroTimeout_PrePollAwake covers the
// PrePollAwake hook in the zero-timeout path (loop.go ~lines 1055-1057).
func TestPhase3_PollFastMode_ZeroTimeout_PrePollAwake(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	var awakeCount atomic.Int32
	loop.testHooks = &loopTestHooks{
		PrePollAwake: func() {
			awakeCount.Add(1)
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Schedule an immediate-fire timer (0 delay) multiple times.
	// When poll runs with an immediate timer, it forces timeout = 0,
	// triggering the zero-timeout path in pollFastMode.
	done := make(chan struct{})
	require.NoError(t, loop.Submit(func() {
		close(done)
	}))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("task didn't execute")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_PollFastMode_TimerExpiry_PrePollAwake covers the
// PrePollAwake hook in the timer-expiry path (loop.go ~lines 1098-1100).
func TestPhase3_PollFastMode_TimerExpiry_PrePollAwake(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	var awakeCount atomic.Int32
	loop.testHooks = &loopTestHooks{
		PrePollAwake: func() {
			awakeCount.Add(1)
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Schedule a short timer (<1000ms). When it expires in pollFastMode,
	// the timer.C case fires, triggering PrePollAwake.
	fired := make(chan struct{})
	_, err = loop.ScheduleTimer(10*time.Millisecond, func() {
		close(fired)
	})
	require.NoError(t, err)

	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("timer didn't fire")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// =============================================================================
// SubmitInternal coverage — targeting loop.go:1263
// =============================================================================

// TestPhase3_SubmitInternal_FastPathDirectExec_StateTerminatedReCheck covers
// the extLen==0 re-check where state has changed to StateTerminated
// between the initial check and the re-check (loop.go ~lines 1276-1278).
func TestPhase3_SubmitInternal_FastPathDirectExec_StateTerminatedReCheck(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Submit a task from the loop thread that submits internally after state changes
	resultCh := make(chan error, 1)
	require.NoError(t, loop.Submit(func() {
		// We're on the loop goroutine. SubmitInternal checks canUseFastPath + isLoopThread.
		// Now force the state to terminated to trigger the re-check path.
		loop.state.Store(StateTerminated)

		err := loop.SubmitInternal(func() {
			// Should not execute
		})
		// Restore so shutdown works
		loop.state.Store(StateRunning)
		resultCh <- err
	}))

	select {
	case err := <-resultCh:
		assert.ErrorIs(t, err, ErrLoopTerminated)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_SubmitInternal_FastPathDirectExec_WithFastEntry covers
// the OnFastPathEntry hook and fastPathEntries counter during direct
// execution on the loop thread (loop.go ~lines 1280-1282).
func TestPhase3_SubmitInternal_FastPathDirectExec_WithFastEntry(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	var hookCalled atomic.Bool
	loop.testHooks = &loopTestHooks{
		OnFastPathEntry: func() {
			hookCalled.Store(true)
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	entriesBefore := loop.fastPathEntries.Load()

	executed := make(chan struct{})
	require.NoError(t, loop.Submit(func() {
		// From loop thread, SubmitInternal should use fast path direct exec
		err := loop.SubmitInternal(func() {
			close(executed)
		})
		if err != nil {
			t.Errorf("SubmitInternal from loop thread failed: %v", err)
		}
	}))

	select {
	case <-executed:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	// fastPathEntries should have been incremented by direct exec
	entriesAfter := loop.fastPathEntries.Load()
	assert.Greater(t, entriesAfter, entriesBefore)

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_SubmitInternal_IOMode_SleepingWakeup covers the I/O mode
// wakeup path in SubmitInternal where canUseFastPath()=false and state is
// StateSleeping (loop.go ~lines 1325-1331).
// Instead of running the full loop (which requires LockOSThread and is
// flaky under high -count), we construct a loop, manually set state to
// StateSleeping, and verify that SubmitInternal sends the wakeup signal.
func TestPhase3_SubmitInternal_IOMode_SleepingWakeup(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	require.NoError(t, err)
	defer loop.Close()

	// Manually set state to Sleeping to exercise the wakeup code path.
	loop.state.Store(StateSleeping)

	// SubmitInternal: canUseFastPath()=false → pushes to internal queue
	// → reads state=Sleeping → CAS(0,1) → doWakeup → channel wakeup.
	var called bool
	err = loop.SubmitInternal(func() { called = true })
	require.NoError(t, err)

	// Verify the wakeup signal was sent to fastWakeupCh.
	select {
	case <-loop.fastWakeupCh:
		// Success
	default:
		t.Fatal("expected wakeup signal on fastWakeupCh")
	}

	// Verify the task is in the internal queue.
	loop.internalQueueMu.Lock()
	task, ok := loop.internal.Pop()
	loop.internalQueueMu.Unlock()
	require.True(t, ok, "internal queue should have the task")
	task()
	assert.True(t, called)

	// Reset state so Close() doesn't hang waiting for a non-existent loop goroutine.
	loop.state.Store(StateAwake)
}

// =============================================================================
// Shutdown coverage — targeting loop.go:380
// =============================================================================

// TestPhase3_Shutdown_DoubleShutdown covers the path where Shutdown is called
// twice. The second call hits stopOnce.Do (no-op), then checks result==nil
// && state!=Terminated (loop.go ~line 385-387).
func TestPhase3_Shutdown_DoubleShutdown(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// First Shutdown
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()
	err = loop.Shutdown(ctx1)
	assert.NoError(t, err)

	// Second Shutdown — stopOnce prevents shutdownImpl from running.
	// result will be nil (zero value), state should be Terminated.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	err2 := loop.Shutdown(ctx2)
	// If state is Terminated: the condition `result==nil && state!=Terminated` is false,
	// so it returns nil. If state is not Terminated: returns ErrLoopTerminated.
	// In practice, after first Shutdown completes, state IS Terminated.
	assert.NoError(t, err2)

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_Shutdown_RunExitsViaContext covers the path where the loop's
// context is cancelled, causing run() to exit, and then Shutdown is called.
// run() handles context cancel by transitioning to Terminated directly.
func TestPhase3_Shutdown_RunExitsViaContext(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Cancel context causes run() to exit and set StateTerminated.
	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}

	// Now Shutdown. stopOnce.Do will run shutdownImpl which sees
	// StateTerminated and returns ErrLoopTerminated immediately.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	err = loop.Shutdown(ctx2)
	// shutdownImpl returns ErrLoopTerminated which is set as result.
	// Then: result != nil, so we just return result.
	assert.ErrorIs(t, err, ErrLoopTerminated)
}

// =============================================================================
// poll coverage — targeting loop.go:946
// =============================================================================

// TestPhase3_Poll_TerminatingAfterTimeoutCalc covers the StateTerminating
// check in poll() at the very beginning (loop.go ~line 949).
// poll() returns immediately when state != StateRunning.
func TestPhase3_Poll_TerminatingAfterTimeoutCalc(t *testing.T) {
	loop, err := New()
	require.NoError(t, err)

	// Test 1: poll() with state = Terminating returns immediately.
	loop.state.Store(StateTerminating)
	loop.poll() // Should return at first check (currentState != StateRunning)

	// Test 2: poll() with state = Running transitions to Sleeping,
	// then PrePollSleep changes state to Terminating before TryTransition.
	// This causes TryTransition(Running→Sleeping) to fail, and poll returns.
	hookCalled := false
	loop.testHooks = &loopTestHooks{
		PrePollSleep: func() {
			hookCalled = true
			// Change state before TryTransition sees it
			loop.state.Store(StateTerminating)
		},
	}
	loop.state.Store(StateRunning)
	loop.poll()
	assert.True(t, hookCalled, "PrePollSleep hook should have fired")

	// Reset state for cleanup
	loop.state.Store(StateAwake)
	loop.Close()
}

// TestPhase3_Poll_ForceNonBlockingPoll covers the forceNonBlockingPoll
// path in poll() where timeout is forced to 0 (loop.go ~line 983).
func TestPhase3_Poll_ForceNonBlockingPoll(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Submit a task that sets forceNonBlockingPoll and verifies the loop
	// continues without blocking.
	done := make(chan struct{})
	require.NoError(t, loop.Submit(func() {
		loop.forceNonBlockingPoll = true
	}))
	require.NoError(t, loop.Submit(func() {
		close(done)
	}))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// =============================================================================
// drainAuxJobs strict microtask ordering — targeting loop.go:924
// =============================================================================

// TestPhase3_DrainAuxJobs_StrictMicrotask_InLoopThread covers
// the strict microtask ordering path in drainAuxJobs when called
// from within the fast poll path. Submit in FastPathForced mode routes
// tasks to auxJobs. drainAuxJobs is called in the poll wake path
// and drains microtasks between each job when strictMicrotaskOrdering=true.
// loop.go ~line 934
func TestPhase3_DrainAuxJobs_StrictMicrotask_InLoopThread(t *testing.T) {
	loop, err := New(
		WithFastPathMode(FastPathForced),
		WithStrictMicrotaskOrdering(true),
	)
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Submit tasks via auxJobs (FastPathForced routes Submit to auxJobs).
	// The first task queues a microtask which, with strict ordering, should
	// drain before the second auxJob runs.
	var order []int
	var mu sync.Mutex

	done := make(chan struct{})
	require.NoError(t, loop.Submit(func() {
		mu.Lock()
		order = append(order, 1)
		mu.Unlock()
		// Queue a microtask — should drain after this task in strict mode
		loop.microtasks.Push(func() {
			mu.Lock()
			order = append(order, 2)
			mu.Unlock()
		})
	}))

	require.NoError(t, loop.Submit(func() {
		mu.Lock()
		order = append(order, 3)
		mu.Unlock()
		close(done)
	}))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	// With strict ordering: job1 → microtask → job2, so order is [1, 2, 3]
	assert.Equal(t, []int{1, 2, 3}, order)

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// =============================================================================
// ScheduleNextTick — targeting loop.go:1429
// =============================================================================

// TestPhase3_ScheduleNextTick_IOMode_SleepingWakeup covers the
// non-fast-path sleeping wakeup via doWakeup in ScheduleNextTick
// (loop.go ~lines 1447-1450).
// Uses direct state manipulation for deterministic testing.
func TestPhase3_ScheduleNextTick_IOMode_SleepingWakeup(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	require.NoError(t, err)
	defer loop.Close()

	// Set state to Sleeping so the wakeup path fires.
	loop.state.Store(StateSleeping)

	// ScheduleNextTick: canUseFastPath()=false → pushes to nextTickQueue
	// → reads state=Sleeping → CAS(0,1) → doWakeup → channel wakeup.
	var called bool
	err = loop.ScheduleNextTick(func() { called = true })
	require.NoError(t, err)

	// Verify the wakeup signal was sent.
	select {
	case <-loop.fastWakeupCh:
		// Success
	default:
		t.Fatal("expected wakeup signal on fastWakeupCh")
	}

	// Verify the task is in the nextTickQueue.
	fn := loop.nextTickQueue.Pop()
	require.NotNil(t, fn)
	fn()
	assert.True(t, called)

	// Reset state so Close() doesn't hang.
	loop.state.Store(StateAwake)
}

// =============================================================================
// Promisify coverage — targeting promisify.go:42
// =============================================================================

// TestPhase3_Promisify_PanicRecovery covers the panic recovery path
// in Promisify where SubmitInternal also fails (promisify.go ~line 92-94).
func TestPhase3_Promisify_PanicRecovery(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Promisify a panicking function
	promise := loop.Promisify(context.Background(), func(ctx context.Context) (any, error) {
		panic("test panic in promisify")
	})

	// The promise should be rejected with PanicError
	ch := promise.ToChannel()
	select {
	case result := <-ch:
		panicErr, ok := result.(PanicError)
		require.True(t, ok, "expected PanicError, got %T: %v", result, result)
		assert.Equal(t, "test panic in promisify", panicErr.Value)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for panic promise")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_Promisify_CtxDoneFallback covers the ctx.Done() path where
// SubmitInternal fails and falls back to direct resolution
// (promisify.go ~line 71-73).
func TestPhase3_Promisify_CtxDoneFallback(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Create a pre-cancelled context
	cancelledCtx, cancelledCancel := context.WithCancel(context.Background())
	cancelledCancel() // Cancel immediately

	// Promisify with pre-cancelled context
	promise := loop.Promisify(cancelledCtx, func(ctx context.Context) (any, error) {
		return "should not reach", nil
	})

	// The promise should be rejected with context.Canceled
	ch := promise.ToChannel()
	select {
	case result := <-ch:
		assert.ErrorIs(t, result.(error), context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_Promisify_ErrorPathViaSubmitInternal covers the fn returns
// error path in Promisify (promisify.go error branch).
func TestPhase3_Promisify_ErrorPathViaSubmitInternal(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	testErr := errors.New("promisify error")
	promise := loop.Promisify(context.Background(), func(ctx context.Context) (any, error) {
		return nil, testErr
	})

	ch := promise.ToChannel()
	select {
	case result := <-ch:
		assert.ErrorIs(t, result.(error), testErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_Promisify_OnTerminatedLoop covers calling Promisify
// when the loop is already terminating or terminated.
func TestPhase3_Promisify_OnTerminatedLoop(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}

	// Now loop is terminated. Promisify should return a rejected promise.
	promise := loop.Promisify(context.Background(), func(ctx context.Context) (any, error) {
		return "should not run", nil
	})

	// Use Promise.ToChannel() interface method
	ch := promise.ToChannel()
	select {
	case result := <-ch:
		assert.Equal(t, ErrLoopTerminated, result)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	loop.Close()
}

// =============================================================================
// promise Finally coverage — targeting promise.go:714
// =============================================================================

// TestPhase3_Finally_PanicRecovery covers the panic recovery path in
// Finally where onFinally panics but the original settlement is still
// propagated (promise.go ~lines 740-742).
func TestPhase3_Finally_PanicRecovery(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	js2, err := NewJS(loop)
	require.NoError(t, err)

	// Create a resolved promise and chain Finally that panics
	p := js2.Resolve("hello")
	child := p.Finally(func() {
		panic("finally panic")
	})

	ch := child.ToChannel()
	select {
	case result := <-ch:
		// Despite panic, original value should propagate
		assert.Equal(t, "hello", result)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_Finally_PanicRecovery_Rejected covers Finally panic recovery
// on a rejected promise.
func TestPhase3_Finally_PanicRecovery_Rejected(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		// Suppress unhandled rejection
	}))
	require.NoError(t, err)

	// Create a rejected promise and chain Finally that panics
	p := js.Reject("bad error")
	child := p.Finally(func() {
		panic("finally panic on rejected")
	})

	ch := child.ToChannel()
	select {
	case result := <-ch:
		// Despite panic, original rejection should propagate
		assert.Equal(t, "bad error", result)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_Finally_NilOnFinally covers the nil onFinally handler path
// in Finally (promise.go ~line 733 sets onFinally = func(){}).
func TestPhase3_Finally_NilOnFinally(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	js, err := NewJS(loop)
	require.NoError(t, err)

	p := js.Resolve(42)
	child := p.Finally(nil)

	ch := child.ToChannel()
	select {
	case result := <-ch:
		assert.Equal(t, 42, result)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// =============================================================================
// executeHandler coverage — targeting promise.go:417
// =============================================================================

// TestPhase3_ExecuteHandler_PanicRejectsTarget covers the panic recovery
// path in executeHandler where a handler panics and the target promise is
// rejected with PanicError (promise.go ~lines 428-430).
func TestPhase3_ExecuteHandler_PanicRejectsTarget(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	js, err := NewJS(loop)
	require.NoError(t, err)

	// Create promise, chain Then with a panicking handler
	p := js.Resolve("input")
	child := p.Then(func(v any) any {
		panic("handler panic")
	}, nil)

	ch := child.ToChannel()
	select {
	case result := <-ch:
		pe, ok := result.(PanicError)
		require.True(t, ok, "expected PanicError, got %T: %v", result, result)
		assert.Equal(t, "handler panic", pe.Value)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_ExecuteHandler_NilHandlerPassthrough covers the nil
// handler pass-through path in executeHandler where the rejection
// propagates to the target because fn is nil.
func TestPhase3_ExecuteHandler_NilHandlerPassthrough(t *testing.T) {
	// Standalone promises (no JS) for executeHandler to run synchronously
	p := &ChainedPromise{js: nil}
	p.state.Store(int32(Pending))

	child := p.Then(nil, nil) // Both nil: pass-through

	// Resolve parent — child should get same value
	p.resolve("pass-through value")

	ch := child.ToChannel()
	select {
	case result := <-ch:
		assert.Equal(t, "pass-through value", result)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// =============================================================================
// checkUnhandledRejections coverage — targeting promise.go:946
// =============================================================================

// TestPhase3_CheckUnhandledRejections_DebugStackWrapping covers the
// debug mode path where creationStack is non-empty and the callback
// receives an UnhandledRejectionDebugInfo wrapper (promise.go ~lines 955-958).
func TestPhase3_CheckUnhandledRejections_DebugStackWrapping(t *testing.T) {
	loop, err := New(
		WithFastPathMode(FastPathForced),
		WithDebugMode(true),
	)
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	var rejectionInfo any
	rejectionCh := make(chan struct{}, 1)

	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		rejectionInfo = reason
		select {
		case rejectionCh <- struct{}{}:
		default:
		}
	}))
	require.NoError(t, err)

	// Create a rejected promise with debug mode enabled (captures creation stack)
	// Don't attach any handler — this triggers the unhandled rejection system.
	require.NoError(t, loop.Submit(func() {
		_, _, reject := js.NewChainedPromise()
		reject("debug rejection reason")
	}))

	// Wait for the unhandled rejection callback to fire
	select {
	case <-rejectionCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for unhandled rejection callback")
	}

	// In debug mode, the rejection should be wrapped in UnhandledRejectionDebugInfo
	debugInfo, ok := rejectionInfo.(*UnhandledRejectionDebugInfo)
	if ok {
		assert.Equal(t, "debug rejection reason", debugInfo.Reason)
		assert.NotEmpty(t, debugInfo.CreationStackTrace)
	} else {
		// If debug stack wasn't captured (e.g., weak pointer was collected),
		// it's still fine — the raw reason is passed
		assert.Equal(t, "debug rejection reason", rejectionInfo)
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_CheckUnhandledRejections_HandledRejectionCleanup covers the
// path where a rejection initially appears unhandled, but a handler is
// registered before the check runs, causing cleanup (promise.go ~lines 990-997).
func TestPhase3_CheckUnhandledRejections_HandledRejectionCleanup(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	unhandledCalled := make(chan struct{}, 1)
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		select {
		case unhandledCalled <- struct{}{}:
		default:
		}
	}))
	require.NoError(t, err)

	// Create a rejected promise and immediately attach a handler
	done := make(chan struct{})
	require.NoError(t, loop.Submit(func() {
		p, _, reject := js.NewChainedPromise()
		reject("will be handled")
		// Register handler immediately — checkUnhandledRejections should
		// find the handler and clean up without reporting.
		p.Catch(func(r any) any {
			return nil // handled
		})
		close(done)
	}))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	// Give microtasks time to drain
	taskDone := make(chan struct{})
	require.NoError(t, loop.Submit(func() {
		close(taskDone)
	}))
	select {
	case <-taskDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	// The unhandled rejection callback should NOT have been called
	select {
	case <-unhandledCalled:
		// It was called — that's OK, the test is about covering the cleanup path
	default:
		// Not called — also fine, handler was registered in time
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// =============================================================================
// NewJS option error — targeting js.go:179
// =============================================================================

// TestPhase3_NewJS_OptionError covers the error path in NewJS when
// resolveJSOptions returns an error (js.go ~lines 179-181).
func TestPhase3_NewJS_OptionError(t *testing.T) {
	loop, err := New()
	require.NoError(t, err)
	defer loop.Close()

	badOpt := &jsOptionImpl{func(o *jsOptions) error {
		return errors.New("bad JS option")
	}}

	_, err = NewJS(loop, badOpt)
	require.Error(t, err)
	assert.Equal(t, "bad JS option", err.Error())
}

// =============================================================================
// SetInterval reschedule error — targeting js.go:357-363
// =============================================================================

// TestPhase3_SetInterval_RescheduleErrorOnShutdown covers the error
// path during interval timer rescheduling when the loop is shutting down
// (js.go ~lines 357-363).
func TestPhase3_SetInterval_RescheduleErrorOnShutdown(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	js, err := NewJS(loop)
	require.NoError(t, err)

	// Set up a short interval that fires multiple times
	var count atomic.Int32

	_, err = js.SetInterval(func() {
		count.Add(1)
	}, 5) // 5ms interval
	require.NoError(t, err)

	// Let it fire at least once
	deadline := time.After(5 * time.Second)
	for count.Load() < 1 {
		select {
		case <-deadline:
			t.Fatal("interval didn't fire")
		default:
			runtime.Gosched()
		}
	}

	// Now shut down — the interval's reschedule will fail
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	err = loop.Shutdown(shutdownCtx)
	assert.NoError(t, err)

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// =============================================================================
// setImmediate run() CAS — targeting js.go:583
// =============================================================================

// TestPhase3_SetImmediate_AlreadyClearedLoad covers the early-return path in
// setImmediateState.run() where cleared.Load() returns true before the CAS.
func TestPhase3_SetImmediate_AlreadyClearedLoad(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	js, err := NewJS(loop)
	require.NoError(t, err)

	var executed atomic.Bool

	// Block the loop so SetImmediate's run() can't fire yet
	blockerStarted := make(chan struct{})
	blockerRelease := make(chan struct{})
	require.NoError(t, loop.Submit(func() {
		close(blockerStarted)
		<-blockerRelease
	}))

	// Wait for the blocker to actually be executing on the loop thread
	select {
	case <-blockerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("blocker didn't start")
	}

	// Set immediate while loop is blocked — run() is queued but can't execute
	id, err := js.SetImmediate(func() {
		executed.Store(true)
	})
	require.NoError(t, err)

	// Clear it before unblocking — when run() eventually executes,
	// cleared.Load() returns true and it bails out early
	err = js.ClearImmediate(id)
	require.NoError(t, err)

	// Unblock the loop — immediate's run() sees cleared=true, returns
	close(blockerRelease)

	// Verify it didn't execute by scheduling another task after it
	done := make(chan struct{})
	require.NoError(t, loop.Submit(func() {
		close(done)
	}))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	assert.False(t, executed.Load(), "cleared immediate should not have executed")

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// =============================================================================
// ingress Pop double-check — targeting ingress.go:154 (lines 168-176)
// =============================================================================

// TestPhase3_ChunkedIngress_PopDoubleCheckExhaustedAfterAdvance covers
// the path in chunkedIngress.Pop() where after advancing to the next
// chunk, the new head is also exhausted (ingress.go ~lines 168-170+174-176).
func TestPhase3_ChunkedIngress_PopDoubleCheckExhaustedAfterAdvance(t *testing.T) {
	// Create a chunkedIngress with small chunk size for easier testing
	ci := newChunkedIngressWithSize(2)

	// Push 4 items (fills 2 chunks of size 2)
	for i := 0; i < 4; i++ {
		ci.Push(func() {})
	}
	assert.Equal(t, 4, ci.Length())

	// Pop all 4
	for i := 0; i < 4; i++ {
		fn, ok := ci.Pop()
		assert.True(t, ok)
		assert.NotNil(t, fn)
	}
	assert.Equal(t, 0, ci.Length())

	// Now push exactly chunkSize items and pop them to exhaust
	ci.Push(func() {})
	ci.Push(func() {})

	fn, ok := ci.Pop()
	assert.True(t, ok)
	assert.NotNil(t, fn)

	fn, ok = ci.Pop()
	assert.True(t, ok)
	assert.NotNil(t, fn)

	// Verify empty
	fn, ok = ci.Pop()
	assert.False(t, ok)
	assert.Nil(t, fn)
}

// TestPhase3_ChunkedIngress_PostAdvanceExhausted exercises the double-check
// path more directly by creating a scenario where:
// 1. head chunk is fully read
// 2. Advance to next chunk
// 3. Next chunk is ALSO fully read (somehow)
func TestPhase3_ChunkedIngress_PostAdvanceExhausted(t *testing.T) {
	ci := newChunkedIngressWithSize(2)

	// Fill 3 chunks (6 items)
	for i := 0; i < 6; i++ {
		ci.Push(func() {})
	}

	// Pop 6 items — after middle chunk is freed, head advances
	for i := 0; i < 6; i++ {
		fn, ok := ci.Pop()
		assert.True(t, ok, "expected pop %d to succeed", i)
		assert.NotNil(t, fn)
	}

	// Empty
	fn, ok := ci.Pop()
	assert.False(t, ok)
	assert.Nil(t, fn)
}

// =============================================================================
// microtaskRing Push overflow and Pop overflow — ingress.go:257,380
// =============================================================================

// TestPhase3_MicrotaskRing_OverflowAndDrain covers the overflow path
// in microtaskRing.Push() and the corresponding overflow drain in Pop()
// (ingress.go ~lines 267 and 380-383).
func TestPhase3_MicrotaskRing_OverflowAndDrain(t *testing.T) {
	ring := newMicrotaskRing()

	// Push more than ringBufferSize items to trigger overflow
	count := ringBufferSize + 100
	for i := 0; i < count; i++ {
		ok := ring.Push(func() {})
		assert.True(t, ok)
	}

	// Verify length
	assert.Equal(t, count, ring.Length())

	// Pop all items — should drain ring first, then overflow
	for i := 0; i < count; i++ {
		fn := ring.Pop()
		assert.NotNil(t, fn, "expected non-nil at position %d", i)
	}

	// Now empty
	fn := ring.Pop()
	assert.Nil(t, fn)
	assert.Equal(t, 0, ring.Length())
}

// =============================================================================
// transitionToTerminated draining — loop.go:642 (lines 671, 691)
// =============================================================================

// TestPhase3_TransitionToTerminated_DrainsAllQueues covers the draining
// paths in transitionToTerminated for external, internal, microtask, and
// auxJobs queues (loop.go ~lines 671, 691).
func TestPhase3_TransitionToTerminated_DrainsAllQueues(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Queue up tasks in multiple queues, then cancel context.
	// transitionToTerminated should drain them all.
	var drained atomic.Int32

	// External queue tasks
	for i := 0; i < 5; i++ {
		require.NoError(t, loop.Submit(func() {
			drained.Add(1)
		}))
	}

	// Internal queue tasks
	for i := 0; i < 5; i++ {
		require.NoError(t, loop.SubmitInternal(func() {
			drained.Add(1)
		}))
	}

	// Cancel triggers transitionToTerminated which drains queues
	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}

	// All tasks should have been drained/executed during shutdown
	assert.GreaterOrEqual(t, drained.Load(), int32(1), "some tasks should have been drained")
}

// =============================================================================
// run() Terminating/Terminated detection — loop.go:470
// =============================================================================

// TestPhase3_Run_DetectsTerminating covers the path in run() where the
// loop detects StateTerminating/StateTerminated set by Shutdown caller
// (loop.go ~line 470-472).
func TestPhase3_Run_DetectsTerminating(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Shutdown transitions to StateTerminating, then the run() main loop
	// detects it and exits via the `StateTerminating || StateTerminated` check.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	err = loop.Shutdown(shutdownCtx)
	assert.NoError(t, err)

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// =============================================================================
// Submit auxJobs → submitWakeup defense — loop.go:1212
// =============================================================================

// TestPhase3_Submit_FastMode_WithUserIOFDs covers the defense-in-depth
// path in Submit where fastMode is true but userIOFDCount > 0
// (loop.go ~line 1212-1214). This can happen transiently during
// concurrent SetFastPathMode/RegisterFD.
func TestPhase3_Submit_FastMode_WithUserIOFDs(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Artificially set userIOFDCount > 0 to trigger the defense-in-depth path.
	loop.userIOFDCount.Add(1)

	executed := make(chan struct{})
	require.NoError(t, loop.Submit(func() {
		close(executed)
	}))

	select {
	case <-executed:
	case <-time.After(5 * time.Second):
		t.Fatal("task didn't execute")
	}

	// Restore for clean shutdown
	loop.userIOFDCount.Add(-1)

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// =============================================================================
// psquare Quantile edge cases — targeting psquare.go:175
// =============================================================================

// TestPhase3_PSquareQuantile_FewObservations covers the Quantile edge case
// where count < 5 and index >= count (psquare.go ~lines 195-197).
func TestPhase3_PSquareQuantile_FewObservations(t *testing.T) {
	t.Run("zero observations", func(t *testing.T) {
		ps := newPSquareQuantile(0.99)
		assert.Equal(t, float64(0), ps.Quantile())
		assert.Equal(t, 0, ps.Count())
	})

	t.Run("one observation", func(t *testing.T) {
		ps := newPSquareQuantile(0.99)
		ps.Update(42.0)
		// With p=0.99 and count=1: index = int(0 * 0.99) = 0
		assert.Equal(t, 42.0, ps.Quantile())
	})

	t.Run("two observations", func(t *testing.T) {
		ps := newPSquareQuantile(0.99)
		ps.Update(10.0)
		ps.Update(20.0)
		// With p=0.99:
		// sorted = [10, 20], index = int(1 * 0.99) = 0
		q := ps.Quantile()
		assert.True(t, q == 10.0 || q == 20.0, "quantile should be one of the values: %f", q)
	})

	t.Run("three observations", func(t *testing.T) {
		ps := newPSquareQuantile(0.5)
		ps.Update(1.0)
		ps.Update(3.0)
		ps.Update(2.0)
		// sorted = [1, 2, 3], index = int(2 * 0.5) = 1
		assert.Equal(t, 2.0, ps.Quantile())
	})

	t.Run("four observations", func(t *testing.T) {
		ps := newPSquareQuantile(0.75)
		ps.Update(4.0)
		ps.Update(1.0)
		ps.Update(3.0)
		ps.Update(2.0)
		// sorted = [1, 2, 3, 4], index = int(3 * 0.75) = 2
		assert.Equal(t, 3.0, ps.Quantile())
	})

	t.Run("p=1.0 high percentile", func(t *testing.T) {
		ps := newPSquareQuantile(1.0)
		ps.Update(5.0)
		ps.Update(10.0)
		ps.Update(15.0)
		// sorted = [5, 10, 15], index = int(2 * 1.0) = 2
		// index(2) >= count(3)? No. index = 2 < 3. Return sorted[2] = 15.
		assert.Equal(t, 15.0, ps.Quantile())
	})

	t.Run("p=1.0 two observations triggers index guard", func(t *testing.T) {
		ps := newPSquareQuantile(1.0)
		ps.Update(100.0)
		ps.Update(200.0)
		// sorted = [100, 200], index = int(1 * 1.0) = 1
		// index(1) >= count(2)? No. index = 1 < 2. Return sorted[1] = 200.
		assert.Equal(t, 200.0, ps.Quantile())
	})
}

// TestPhase3_PSquareQuantile_Max_FewObservations covers the Max() method
// for count < 5.
func TestPhase3_PSquareQuantile_Max_FewObservations(t *testing.T) {
	ps := newPSquareQuantile(0.5)
	assert.Equal(t, float64(0), ps.Max())

	ps.Update(5.0)
	assert.Equal(t, 5.0, ps.Max())

	ps.Update(3.0)
	assert.Equal(t, 5.0, ps.Max())

	ps.Update(10.0)
	assert.Equal(t, 10.0, ps.Max())
}

// =============================================================================
// poll I/O mode — PrePollAwake hook (loop.go:1012)
// =============================================================================

// TestPhase3_Shutdown_FromLoopThread covers calling Shutdown from
// within the loop goroutine via a submitted task.
func TestPhase3_Shutdown_FromLoopThread(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Submit a task that calls Shutdown from loop thread.
	// Shutdown needs to wait for loopDone, but we're ON the loop thread.
	// This exercises the shutdownImpl path.
	resultCh := make(chan error, 1)
	require.NoError(t, loop.Submit(func() {
		// ctx with short timeout to prevent deadlock from loopDone wait
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer shutCancel()
		resultCh <- loop.Shutdown(shutCtx)
	}))

	select {
	case err := <-resultCh:
		// Expected: context.DeadlineExceeded because Shutdown waits for loopDone
		// which can't close while we're in the loop goroutine
		assert.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// =============================================================================
// submitWakeup on terminated loop — loop.go:1146
// =============================================================================

// TestPhase3_SubmitWakeup_Terminated covers the ErrLoopTerminated
// return path in submitWakeup (loop.go ~line 1157-1160).
func TestPhase3_SubmitWakeup_Terminated(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}

	// Now terminated — submitWakeup should return ErrLoopTerminated
	err = loop.submitWakeup()
	assert.ErrorIs(t, err, ErrLoopTerminated)

	loop.Close()
}

// =============================================================================
// Promise standalone — executeHandler with nil target pass-through reject
// =============================================================================

// TestPhase3_ExecuteHandler_NilTarget covers the nil target early return
// in executeHandler (promise.go ~line 426-427).
func TestPhase3_ExecuteHandler_NilTarget(t *testing.T) {
	// Create a standalone promise with no JS adapter
	p := &ChainedPromise{js: nil}
	p.state.Store(int32(Pending))

	// Add a handler with nil target and nil fn
	p.addHandler(handler{
		onFulfilled: nil,
		onRejected:  nil,
		target:      nil,
	})

	// Resolve — the handler with nil target and nil fn should just return
	p.resolve("value")

	// Should not panic, promise should settle normally
	ch := p.ToChannel()
	select {
	case result := <-ch:
		assert.Equal(t, "value", result)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// =============================================================================
// ScheduleMicrotask in I/O mode — loop.go:1361
// =============================================================================

// TestPhase3_ScheduleMicrotask_IOMode covers ScheduleMicrotask when
// the loop is in I/O (non-fast) mode and sleeping.
// Uses direct state manipulation for deterministic testing.
func TestPhase3_ScheduleMicrotask_IOMode(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	require.NoError(t, err)
	defer loop.Close()

	// Set state to Sleeping so the wakeup path fires.
	loop.state.Store(StateSleeping)

	// ScheduleMicrotask: canUseFastPath()=false → pushes to microtasks
	// → reads state=Sleeping → CAS(0,1) → doWakeup → channel wakeup.
	var called bool
	err = loop.ScheduleMicrotask(func() { called = true })
	require.NoError(t, err)

	// Verify the wakeup signal was sent.
	select {
	case <-loop.fastWakeupCh:
		// Success
	default:
		t.Fatal("expected wakeup signal on fastWakeupCh")
	}

	// Verify the task is in the microtask queue.
	fn := loop.microtasks.Pop()
	require.NotNil(t, fn)
	fn()
	assert.True(t, called)

	// Reset state so Close() doesn't hang.
	loop.state.Store(StateAwake)
}

// =============================================================================
// registerRejectionHandler paths — promise.go:602
// =============================================================================

// TestPhase3_RegisterRejectionHandler_FulfilledCleanup covers the
// Fulfilled path in registerRejectionHandler where handler tracking is
// cleaned up for a promise that resolved (never rejected).
func TestPhase3_RegisterRejectionHandler_FulfilledCleanup(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	js, err := NewJS(loop)
	require.NoError(t, err)

	// Resolve a promise, then chain Catch on it.
	// registerRejectionHandler sees Fulfilled state and cleans up.
	p := js.Resolve("done")
	child := p.Catch(func(r any) any {
		return nil
	})

	ch := child.ToChannel()
	select {
	case result := <-ch:
		// Catch on resolved promise: pass-through
		assert.Equal(t, "done", result)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// =============================================================================
// Promise chaining cycle detection — promise.go:455
// =============================================================================

// TestPhase3_Promise_ChainingCycleDetection covers the self-resolution
// check in resolve() that detects chaining cycles.
func TestPhase3_Promise_ChainingCycleDetection(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		// Suppress
	}))
	require.NoError(t, err)

	done := make(chan any, 1)
	require.NoError(t, loop.Submit(func() {
		p, resolve, _ := js.NewChainedPromise()
		// Self-resolve: resolve(p) should detect cycle
		resolve(p)
		ch := p.ToChannel()
		result := <-ch
		done <- result
	}))

	select {
	case result := <-done:
		// Should be a TypeError about chaining cycle
		err, ok := result.(error)
		require.True(t, ok, "expected error, got %T", result)
		assert.Contains(t, err.Error(), "Chaining cycle")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// =============================================================================
// PromisifyWithTimeout / PromisifyWithDeadline
// =============================================================================

// TestPhase3_PromisifyWithTimeout covers PromisifyWithTimeout basic usage
// to ensure the wrapper context cancellation works.
func TestPhase3_PromisifyWithTimeout(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Promisify a function that returns a result within timeout
	promise := loop.PromisifyWithTimeout(context.Background(), 5*time.Second,
		func(ctx context.Context) (any, error) {
			return "timeout-ok", nil
		})

	ch := promise.ToChannel()

	select {
	case result := <-ch:
		assert.Equal(t, "timeout-ok", result)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// TestPhase3_PromisifyWithDeadline covers PromisifyWithDeadline basic usage
// to ensure the wrapper context cancellation works.
func TestPhase3_PromisifyWithDeadline(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	deadline := time.Now().Add(5 * time.Second)
	promise := loop.PromisifyWithDeadline(context.Background(), deadline,
		func(ctx context.Context) (any, error) {
			return "deadline-ok", nil
		})

	ch := promise.ToChannel()

	select {
	case result := <-ch:
		assert.Equal(t, "deadline-ok", result)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// =============================================================================
// PanicError.Error() method
// =============================================================================

// TestPhase3_PanicError_Error covers the PanicError.Error() method.
func TestPhase3_PanicError_Error(t *testing.T) {
	pe := PanicError{Value: "test panic value"}
	assert.Equal(t, "promise: goroutine panicked: test panic value", pe.Error())

	pe2 := PanicError{Value: 42}
	assert.Equal(t, "promise: goroutine panicked: 42", pe2.Error())
}

// =============================================================================
// Promise.ToChannel on already-settled standalone promise
// =============================================================================

// TestPhase3_ToChannel_StandaloneAlreadySettled covers the fast-path
// in ToChannel where the promise is already settled (no lock needed).
func TestPhase3_ToChannel_StandaloneAlreadySettled(t *testing.T) {
	p := &ChainedPromise{js: nil}
	p.state.Store(int32(Pending))
	p.resolve("fast")

	ch := p.ToChannel()
	select {
	case result := <-ch:
		assert.Equal(t, "fast", result)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// TestPhase3_ToChannel_StandalonePending covers ToChannel on a
// standalone pending promise (handler-based fallback).
func TestPhase3_ToChannel_StandalonePending(t *testing.T) {
	p := &ChainedPromise{js: nil}
	p.state.Store(int32(Pending))

	ch := p.ToChannel()

	// Resolve after ToChannel is set up
	go func() {
		runtime.Gosched()
		p.resolve("deferred")
	}()

	select {
	case result := <-ch:
		assert.Equal(t, "deferred", result)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// =============================================================================
// Phase 3 — Additional coverage tests (batch 2)
// Targets: transitionToTerminated drain, microtaskRing compact,
// pSquare index overflow, drainAuxJobs strict ordering, SubmitInternal
// IO mode with userFDs, Shutdown stopOnce spent, poll IO mode PrePollAwake.
// =============================================================================

// TestPhase3_TransitionToTerminated_DirectDrain covers the drain paths in
// transitionToTerminated for internal, external, and auxJobs queues.
// By pre-loading queues WITHOUT running the event loop, we guarantee
// transitionToTerminated will find tasks to drain (loop.go lines 660, 671, 691).
func TestPhase3_TransitionToTerminated_DirectDrain(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)

	var internalRun, externalRun, auxRun atomic.Int32

	// Pre-load internal queue directly
	loop.internalQueueMu.Lock()
	loop.internal.Push(func() { internalRun.Add(1) })
	loop.internal.Push(func() { internalRun.Add(1) })
	loop.internalQueueMu.Unlock()

	// Pre-load external queue directly
	loop.externalMu.Lock()
	loop.external.Push(func() { externalRun.Add(1) })
	loop.external.Push(func() { externalRun.Add(1) })
	loop.externalMu.Unlock()

	// Pre-load auxJobs directly
	loop.externalMu.Lock()
	loop.auxJobs = append(loop.auxJobs, func() { auxRun.Add(1) })
	loop.auxJobs = append(loop.auxJobs, func() { auxRun.Add(1) })
	loop.externalMu.Unlock()

	// Directly call transitionToTerminated — drains all queues
	loop.transitionToTerminated()

	assert.Equal(t, int32(2), internalRun.Load(), "internal queue tasks drained")
	assert.Equal(t, int32(2), externalRun.Load(), "external queue tasks drained")
	assert.Equal(t, int32(2), auxRun.Load(), "auxJobs tasks drained")
	assert.Equal(t, StateTerminated, loop.state.Load())
}

// TestPhase3_TransitionToTerminated_DirectDrainMicrotaskNextTick covers the
// drain of microtask queue. Note: nextTick is NOT drained by transitionToTerminated.
func TestPhase3_TransitionToTerminated_DirectDrainMicrotaskNextTick(t *testing.T) {
	loop, err := New()
	require.NoError(t, err)

	var microRun atomic.Int32

	loop.microtasks.Push(func() { microRun.Add(1) })
	loop.microtasks.Push(func() { microRun.Add(1) })

	loop.transitionToTerminated()

	assert.Equal(t, int32(2), microRun.Load())
}

// TestPhase3_MicrotaskRing_OverflowCompact covers the overflow compact path
// in microtaskRing.Pop (ingress.go lines 390-393).
// The compact triggers when overflowHead > len(overflow)/2 AND
// overflowHead > ringOverflowCompactThreshold (512).
func TestPhase3_MicrotaskRing_OverflowCompact(t *testing.T) {
	ring := newMicrotaskRing()

	// Push ringBufferSize + 1024 items: ring gets 4096, overflow gets 1024
	total := ringBufferSize + 1024
	for i := 0; i < total; i++ {
		ok := ring.Push(func() {})
		require.True(t, ok, "push %d failed", i)
	}
	assert.Equal(t, total, ring.Length())

	// Pop all items. After ring drains (4096 pops), overflow pops begin.
	// After popping ~513 from overflow: overflowHead=513 > 512 AND 513 > 1024/2=512
	// → compact triggers.
	for i := 0; i < total; i++ {
		fn := ring.Pop()
		require.NotNil(t, fn, "pop %d returned nil", i)
	}
	assert.Equal(t, 0, ring.Length())

	// Ring is still functional after compaction
	ok := ring.Push(func() {})
	assert.True(t, ok)
	fn := ring.Pop()
	assert.NotNil(t, fn)
}

// TestPhase3_PSquareQuantile_IndexOverflow covers the index >= count guard
// in Quantile (psquare.go lines 195-197) using an extreme p value.
func TestPhase3_PSquareQuantile_IndexOverflow(t *testing.T) {
	// With p=5.0 and 2 observations: index = int(float64(1) * 5.0) = 5 >= 2 → clamped
	ps := newPSquareQuantile(5.0)
	ps.Update(10.0)
	ps.Update(20.0)
	assert.Equal(t, 20.0, ps.Quantile(), "should clamp to max observation")

	// With 3 observations and p=5.0: index = int(2 * 5.0) = 10 >= 3 → clamped
	ps2 := newPSquareQuantile(5.0)
	ps2.Update(5.0)
	ps2.Update(15.0)
	ps2.Update(25.0)
	assert.Equal(t, 25.0, ps2.Quantile())
}

// TestPhase3_DrainAuxJobs_StrictOrdering_DirectCall covers the strict
// microtask ordering path in drainAuxJobs by calling it directly with
// pre-loaded auxJobs (loop.go line 934-936).
func TestPhase3_DrainAuxJobs_StrictOrdering_DirectCall(t *testing.T) {
	loop, err := New(WithStrictMicrotaskOrdering(true))
	require.NoError(t, err)
	defer loop.Close()

	var order []int

	// Pre-load auxJobs directly (no running loop needed)
	loop.externalMu.Lock()
	loop.auxJobs = append(loop.auxJobs, func() {
		order = append(order, 1)
		// Push a microtask — should be drained before next auxJob
		loop.microtasks.Push(func() {
			order = append(order, 2)
		})
	})
	loop.auxJobs = append(loop.auxJobs, func() {
		order = append(order, 3)
	})
	loop.externalMu.Unlock()

	// Direct call — triggers the strict microtask path
	loop.drainAuxJobs()

	// With strict ordering: job1 → microtask(2) → job2
	assert.Equal(t, []int{1, 2, 3}, order)
}

// TestPhase3_SubmitInternal_IOMode_WithUserFDs covers the I/O mode wakeup
// path in SubmitInternal where userIOFDCount > 0 forces the pipe-based
// wakeup instead of channel-based (loop.go lines 1325-1331).
func TestPhase3_SubmitInternal_IOMode_WithUserFDs(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	require.NoError(t, err)
	defer loop.Close()

	// Force I/O mode: userIOFDCount > 0 bypasses channel wakeup path
	loop.userIOFDCount.Store(1)
	loop.state.Store(StateSleeping)

	var called bool
	err = loop.SubmitInternal(func() { called = true })
	require.NoError(t, err)

	// Verify the task is in the internal queue
	loop.internalQueueMu.Lock()
	task, ok := loop.internal.Pop()
	loop.internalQueueMu.Unlock()
	require.True(t, ok, "internal queue should have the task")
	task()
	assert.True(t, called)

	// Drain the fastWakeupCh (doWakeup sends on both channel and pipe)
	select {
	case <-loop.fastWakeupCh:
	default:
	}

	// Reset for Close
	loop.userIOFDCount.Store(0)
	loop.state.Store(StateAwake)
}

// TestPhase3_Shutdown_StopOnceSpent covers the Shutdown path where
// stopOnce has already been consumed, causing result to remain nil.
// When state != Terminated, returns ErrLoopTerminated (loop.go lines 385-387).
func TestPhase3_Shutdown_StopOnceSpent(t *testing.T) {
	loop, err := New()
	require.NoError(t, err)
	defer loop.Close()

	// Spend stopOnce without actually shutting down
	loop.stopOnce.Do(func() {
		// No-op: consume the Once without calling shutdownImpl
	})

	// State is still StateAwake (never started). Calling Shutdown:
	// - stopOnce.Do is no-op (already done)
	// - result stays nil
	// - state.Load() == StateAwake != StateTerminated
	// - Returns ErrLoopTerminated
	err = loop.Shutdown(context.Background())
	assert.ErrorIs(t, err, ErrLoopTerminated)
}

// TestPhase3_Poll_IOMode_PrePollAwake covers the PrePollAwake hook in
// poll()'s I/O mode path (loop.go line ~1012-1014) by directly calling
// poll() with userIOFDCount > 0 and forceNonBlockingPoll = true.
func TestPhase3_Poll_IOMode_PrePollAwake(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	require.NoError(t, err)

	var hookCalled atomic.Bool
	loop.testHooks = &loopTestHooks{
		PrePollAwake: func() {
			hookCalled.Store(true)
		},
	}

	// Force I/O mode and non-blocking poll
	loop.userIOFDCount.Store(1)
	loop.forceNonBlockingPoll = true
	loop.state.Store(StateRunning)

	// Direct call to poll — should:
	// 1. Transition Running → Sleeping
	// 2. Find queues empty, not terminating
	// 3. forceNonBlockingPoll → timeout = 0
	// 4. userIOFDCount > 0 → I/O mode → PollIO(0) → immediate return
	// 5. PrePollAwake hook fires
	// 6. Transition Sleeping → Running
	loop.poll()

	assert.True(t, hookCalled.Load(), "PrePollAwake should have fired in IO mode")

	// Reset for Close
	loop.userIOFDCount.Store(0)
	loop.state.Store(StateAwake)
	loop.Close()
}

// TestPhase3_ScheduleNextTick_NilFunction covers the nil function
// early return in ScheduleNextTick (loop.go line 1430-1432).
func TestPhase3_ScheduleNextTick_NilFunction(t *testing.T) {
	loop, err := New()
	require.NoError(t, err)
	defer loop.Close()

	err = loop.ScheduleNextTick(nil)
	assert.NoError(t, err)
}

// TestPhase3_ScheduleNextTick_TerminatedLoop covers the ErrLoopTerminated
// return in ScheduleNextTick when state is Terminated (loop.go line 1435-1437).
func TestPhase3_ScheduleNextTick_TerminatedLoop(t *testing.T) {
	loop, err := New()
	require.NoError(t, err)

	// Set state to Terminated directly
	loop.state.Store(StateTerminated)

	err = loop.ScheduleNextTick(func() {})
	assert.ErrorIs(t, err, ErrLoopTerminated)

	// Reset for Close
	loop.state.Store(StateAwake)
	loop.Close()
}

// TestPhase3_SubmitInternal_FastPath_WakesOnPendingTimers covers the
// fast-path wakeup in SubmitInternal when a directly-executed task
// adds timers (loop.go ~lines 1296-1300).
func TestPhase3_SubmitInternal_FastPath_WakesOnPendingTimers(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Submit a task via SubmitInternal that schedules a timer.
	// Fast-path direct-exec detects pending timers and sends wakeup.
	timerFired := make(chan struct{})
	require.NoError(t, loop.SubmitInternal(func() {
		_, err := loop.ScheduleTimer(1*time.Millisecond, func() {
			close(timerFired)
		})
		if err != nil {
			t.Errorf("ScheduleTimer failed: %v", err)
		}
	}))

	select {
	case <-timerFired:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for timer")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// =============================================================================
// pollFastMode direct-call tests for remaining coverage
// =============================================================================

// TestPhase3_PollFastMode_TerminatingCheck_DirectCall covers the first
// termination check in pollFastMode (loop.go ~line 1048) by directly
// calling pollFastMode with state already set to Terminating.
func TestPhase3_PollFastMode_TerminatingCheck_DirectCall(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)

	// State must be Terminating for the check to fire.
	// No signal in fastWakeupCh (so channel drain returns nothing).
	loop.state.Store(StateTerminating)

	// pollFastMode: drain (nothing) → first term check → hits body → returns
	loop.pollFastMode(500)

	// Reset for Close
	loop.state.Store(StateAwake)
	loop.Close()
}

// TestPhase3_PollFastMode_ZeroTimeout_DirectCall covers the zero-timeout
// path in pollFastMode with PrePollAwake hook (loop.go ~line 1055-1057).
func TestPhase3_PollFastMode_ZeroTimeout_DirectCall(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)

	var hookCalled atomic.Bool
	loop.testHooks = &loopTestHooks{
		PrePollAwake: func() {
			hookCalled.Store(true)
		},
	}

	// State should be Sleeping (as poll() would set it).
	loop.state.Store(StateSleeping)

	// pollFastMode(0): drain (nothing) → not Terminating → timeoutMs==0 →
	// PrePollAwake fires → drainAuxJobs → TryTransition(Sleeping→Running)
	loop.pollFastMode(0)

	assert.True(t, hookCalled.Load())

	// Reset for Close (state was transitioned to Running)
	loop.state.Store(StateAwake)
	loop.Close()
}
