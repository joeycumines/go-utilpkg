package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestAutoExit_DefaultDisabled verifies that without WithAutoExit,
// the loop does NOT exit on its own when no work remains. This proves
// backward compatibility — the default behavior is unchanged.
func TestAutoExit_DefaultDisabled(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()

	// Schedule a short timer that fires quickly.
	fired := make(chan struct{})
	_, err = loop.ScheduleTimer(5*time.Millisecond, func() {
		close(fired)
	})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Wait for the timer to fire.
	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("timer should have fired by now")
	}

	// Wait a bit and confirm the loop is STILL running (did not auto-exit).
	time.Sleep(50 * time.Millisecond)
	if state := loop.State(); state == StateTerminated {
		t.Error("loop should NOT have auto-exited without WithAutoExit")
	}

	// Clean up.
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

// TestAutoExit_ImmediateExit verifies that a loop with WithAutoExit(true)
// and no initial work exits Run() immediately with nil error.
func TestAutoExit_ImmediateExit(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
		if state := loop.State(); state != StateTerminated {
			t.Errorf("loop state = %v, want StateTerminated", state)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within timeout — loop should have exited immediately")
	}
}

// TestAutoExit_AfterTimerFires verifies that the loop exits after a ref'd
// timer fires and no other ref'd work remains.
func TestAutoExit_AfterTimerFires(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fired := make(chan struct{})
	_, err = loop.ScheduleTimer(10*time.Millisecond, func() {
		close(fired)
	})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()

	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("timer should have fired")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after timer fired — loop should have auto-exited")
	}
}

// TestAutoExit_UnrefTimerCausesExit verifies that unref'ing the only ref'd
// timer causes the loop to exit.
func TestAutoExit_UnrefTimerCausesExit(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Schedule a long timer, then unref it.
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()

	// Wait for timer to be registered.
	time.Sleep(20 * time.Millisecond)

	// Unref the timer — loop should exit because no ref'd work remains.
	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after UnrefTimer — loop should have auto-exited")
	}
}

func TestAutoExit_UnrefTimerCleansTimerState(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()
	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after UnrefTimer")
	}

	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount should be 0 after auto-exit cleanup, got %d", got)
	}
	if got := len(loop.timerMap); got != 0 {
		t.Fatalf("timerMap should be empty after auto-exit cleanup, got %d entries", got)
	}
	if got := len(loop.timers); got != 0 {
		t.Fatalf("timers heap should be empty after auto-exit cleanup, got %d entries", got)
	}
}

// TestAutoExit_RefTimerPreventsExit verifies that ref'ing a previously
// unref'd timer prevents the loop from exiting. Uses a blocking callback
// to hold the loop alive while we do the unref+ref cycle.
func TestAutoExit_RefTimerPreventsExit(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Schedule a long timer — it's ref'd by default.
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Use a blocking Submit to hold the loop while we manipulate ref state.
	proceed := make(chan struct{})
	var refDone atomic.Bool
	err = loop.Submit(func() {
		// On the loop thread: unref and re-ref synchronously.
		loop.UnrefTimer(timerID) // synchronous on loop thread
		loop.RefTimer(timerID)   // synchronous on loop thread
		refDone.Store(true)
		<-proceed
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()

	// Wait for the unref+ref to complete.
	time.Sleep(30 * time.Millisecond)
	if !refDone.Load() {
		t.Fatal("unref+ref should have completed")
	}

	// Loop is still alive because the Submit callback is blocking + timer is ref'd.
	if state := loop.State(); state == StateTerminated {
		t.Error("loop should NOT have exited — timer is ref'd again")
	}

	// Unblock the callback. Loop stays alive because the timer is still ref'd.
	close(proceed)

	// Wait a bit and confirm loop is STILL running.
	time.Sleep(50 * time.Millisecond)
	if state := loop.State(); state == StateTerminated {
		t.Error("loop should NOT have exited after unblock — timer is still ref'd")
	}

	// Clean up.
	_ = loop.Close()
	<-done // drain
}

// TestAutoExit_ExternalTaskPreventsExit verifies that pending external tasks
// prevent the loop from auto-exiting.
func TestAutoExit_ExternalTaskPreventsExit(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	blockDone := make(chan struct{})

	// Submit the blocking task BEFORE starting Run() to avoid the auto-exit race.
	var taskRan atomic.Bool
	err = loop.Submit(func() {
		taskRan.Store(true)
		<-blockDone
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()

	// Wait for task to start.
	time.Sleep(20 * time.Millisecond)
	if !taskRan.Load() {
		t.Fatal("task should have started")
	}

	// Loop should still be running (executing the blocking task).
	if state := loop.State(); state == StateTerminated {
		t.Error("loop should not exit while executing a task")
	}

	// Unblock the task — now the loop should auto-exit (no more work).
	close(blockDone)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after task completed")
	}
}

// TestAutoExit_PromisifyPreventsExit verifies that in-flight Promisify
// goroutines prevent the loop from auto-exiting.
func TestAutoExit_PromisifyPreventsExit(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	unblock := make(chan struct{})

	// Start Promisify BEFORE Run() to ensure the loop sees it as alive.
	promiseRan := make(chan struct{})
	_ = loop.Promisify(context.Background(), func(_ context.Context) (any, error) {
		close(promiseRan)
		<-unblock
		return 42, nil
	})

	// Wait for Promisify goroutine to start.
	select {
	case <-promiseRan:
	case <-time.After(2 * time.Second):
		t.Fatal("Promisify goroutine should have started")
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()

	// Loop should be alive due to Promisify goroutine.
	time.Sleep(20 * time.Millisecond)
	if !loop.Alive() {
		t.Error("Alive() should be true with in-flight Promisify goroutine")
	}

	// Unblock the Promisify goroutine — loop should auto-exit.
	close(unblock)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Promisify completed")
	}
}

// TestAutoExit_FastPathMode verifies that auto-exit works correctly
// in FastPathForced mode.
func TestAutoExit_FastPathMode(t *testing.T) {
	loop, err := New(
		WithAutoExit(true),
		WithFastPathMode(FastPathForced),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fired := make(chan struct{})
	_, err = loop.ScheduleTimer(10*time.Millisecond, func() {
		close(fired)
	})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()

	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("timer should have fired")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return in fast path mode")
	}
}

// TestAutoExit_PollPathMode verifies that auto-exit works correctly
// in FastPathDisabled (poll) mode.
func TestAutoExit_PollPathMode(t *testing.T) {
	loop, err := New(
		WithAutoExit(true),
		WithFastPathMode(FastPathDisabled),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fired := make(chan struct{})
	_, err = loop.ScheduleTimer(10*time.Millisecond, func() {
		close(fired)
	})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()

	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("timer should have fired")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return in poll path mode")
	}
}

// TestAutoExit_MicrotaskPreventsExit verifies that pending microtasks
// prevent the loop from auto-exiting.
func TestAutoExit_MicrotaskPreventsExit(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	blockDone := make(chan struct{})

	// Submit a blocking task before Run() starts. Schedule a microtask from within.
	var microRan atomic.Bool
	err = loop.Submit(func() {
		loop.ScheduleMicrotask(func() {
			microRan.Store(true)
		})
		<-blockDone
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()

	time.Sleep(20 * time.Millisecond)

	// Loop should be alive (executing the blocking task, microtask pending).
	if !loop.Alive() {
		t.Error("Alive() should be true while task is executing")
	}

	// Unblock — microtask will run, then loop should auto-exit.
	close(blockDone)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
		if !microRan.Load() {
			t.Error("microtask should have run")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after microtask completed")
	}
}

// TestAutoExit_NextTickPreventsExit verifies that pending nextTick callbacks
// prevent the loop from auto-exiting.
func TestAutoExit_NextTickPreventsExit(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	blockDone := make(chan struct{})

	var tickRan atomic.Bool
	err = loop.Submit(func() {
		loop.ScheduleNextTick(func() {
			tickRan.Store(true)
		})
		<-blockDone
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()

	time.Sleep(20 * time.Millisecond)

	close(blockDone)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
		if !tickRan.Load() {
			t.Error("nextTick callback should have run")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after nextTick completed")
	}
}

func TestAutoExit_TerminationDrainsAcceptedNextTick(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	enteredTermination := make(chan struct{})
	releaseTermination := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		BeforeTerminateState: func() {
			close(enteredTermination)
			<-releaseTermination
		},
	}

	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()
	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	select {
	case <-enteredTermination:
	case <-time.After(5 * time.Second):
		t.Fatal("termination hook did not trigger")
	}

	var ran atomic.Bool
	if err := loop.ScheduleNextTick(func() { ran.Store(true) }); err != nil {
		t.Fatalf("ScheduleNextTick during termination: %v", err)
	}

	close(releaseTermination)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after draining nextTick")
	}

	if !ran.Load() {
		t.Fatal("nextTick callback should have run during termination drain")
	}
	if !loop.nextTickQueue.IsEmpty() {
		t.Fatal("nextTickQueue should be empty after termination drain")
	}
}

// TestAutoExit_MixedWorkloads verifies that the loop exits only when ALL
// ref'd work types are complete: timer + Submit + Promisify together.
func TestAutoExit_MixedWorkloads(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var timerFired, taskRan, promisified atomic.Bool
	unblockPromisify := make(chan struct{})

	// Schedule a short timer.
	_, err = loop.ScheduleTimer(10*time.Millisecond, func() {
		timerFired.Store(true)
	})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Submit a task that starts a Promisify.
	err = loop.Submit(func() {
		taskRan.Store(true)
		loop.Promisify(context.Background(), func(_ context.Context) (any, error) {
			<-unblockPromisify
			promisified.Store(true)
			return "done", nil
		})
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()

	// Wait for the timer and task to execute.
	time.Sleep(50 * time.Millisecond)

	if !timerFired.Load() {
		t.Error("timer should have fired")
	}
	if !taskRan.Load() {
		t.Error("task should have run")
	}

	// Loop should still be alive due to Promisify goroutine.
	if loop.State() == StateTerminated {
		t.Error("loop should not exit while Promisify goroutine is in-flight")
	}

	// Unblock Promisify — loop should auto-exit now.
	close(unblockPromisify)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
		if !promisified.Load() {
			t.Error("Promisify should have completed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after all work completed")
	}
}

// TestAutoExit_IntervalReschedule verifies auto-exit behavior with intervals.
// When the interval is unref'd, the loop should exit.
func TestAutoExit_IntervalReschedule(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS: %v", err)
	}

	var fireCount atomic.Int32
	intervalID, err := js.SetInterval(func() {
		fireCount.Add(1)
	}, 20)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()

	// Wait for a few interval fires.
	time.Sleep(100 * time.Millisecond)
	count := fireCount.Load()
	if count < 2 {
		t.Errorf("expected at least 2 interval fires, got %d", count)
	}

	// Unref the interval — loop should exit.
	js.UnrefInterval(intervalID)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after UnrefInterval")
	}
}

// TestAutoExit_ConcurrentSubmit verifies that auto-exit is safe under
// concurrent work submission: no work is lost when the loop decides to exit.
func TestAutoExit_ConcurrentSubmit(t *testing.T) {
	const numTasks = 100

	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var completed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numTasks)

	// Submit all tasks before starting the loop.
	for i := 0; i < numTasks; i++ {
		err := loop.Submit(func() {
			completed.Add(1)
			wg.Done()
		})
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()

	// All tasks should complete before auto-exit.
	wg.Wait()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
		finalCount := completed.Load()
		if finalCount != numTasks {
			t.Errorf("expected %d completed tasks, got %d", numTasks, finalCount)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("Run did not return, completed %d/%d tasks", completed.Load(), numTasks)
	}
}

// TestAutoExit_OptionType verifies the AutoExitOption concrete type
// satisfies the LoopOption interface at runtime.
func TestAutoExit_OptionType(t *testing.T) {
	opt := WithAutoExit(true)

	// Verify concrete type is *AutoExitOption.
	var loopOpt LoopOption = opt
	if _, ok := loopOpt.(*AutoExitOption); !ok {
		t.Errorf("WithAutoExit should return *AutoExitOption, got %T", opt)
	}

	// Verify it implements LoopOption by applying it.
	cfg := &loopOptions{}
	if err := opt.applyLoop(cfg); err != nil {
		t.Fatalf("applyLoop: %v", err)
	}
	if !cfg.autoExit {
		t.Error("autoExit should be true")
	}

	// Verify disabled case.
	optDisabled := WithAutoExit(false)
	if err := optDisabled.applyLoop(cfg); err != nil {
		t.Fatalf("applyLoop: %v", err)
	}
	if cfg.autoExit {
		t.Error("autoExit should be false")
	}
}

// TestAutoExit_NilOption verifies that WithAutoExit works with nil-safe
// resolveLoopOptions (nil options are skipped).
func TestAutoExit_NilOption(t *testing.T) {
	opts := []LoopOption{nil, WithAutoExit(true), nil}
	cfg, err := resolveLoopOptions(opts)
	if err != nil {
		t.Fatalf("resolveLoopOptions: %v", err)
	}
	if !cfg.autoExit {
		t.Error("autoExit should be true despite nil options")
	}
}

// TestAutoExit_UnrefTimerFullLifecycle exercises the complete auto-exit + RefTimer/UnrefTimer
// lifecycle end-to-end: schedule, unref (triggers exit), verify termination state, then
// verify RefTimer correctly rejects on the terminated loop.
func TestAutoExit_UnrefTimerFullLifecycle(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Schedule a timer to keep the loop alive.
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	// Wait for the timer to be registered.
	time.Sleep(20 * time.Millisecond)

	// Verify the loop is alive.
	if !loop.Alive() {
		t.Fatal("Alive() should be true with a ref'd timer")
	}

	// Unref the timer — loop should exit.
	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	// Wait for Run() to return.
	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after UnrefTimer — loop should have auto-exited")
	}

	// Verify post-termination state.
	if loop.Alive() {
		t.Fatal("Alive() should be false after auto-exit")
	}
	if loop.State() != StateTerminated {
		t.Fatalf("State should be StateTerminated, got %v", loop.State())
	}

	// Verify RefTimer rejects on terminated loop.
	refErr := loop.RefTimer(timerID)
	if refErr == nil {
		t.Fatal("RefTimer should return error on terminated loop")
	}
	if refErr != ErrLoopTerminated {
		t.Fatalf("RefTimer should return ErrLoopTerminated, got %v", refErr)
	}

	// Verify UnrefTimer also rejects on terminated loop.
	unrefErr := loop.UnrefTimer(timerID)
	if unrefErr == nil {
		t.Fatal("UnrefTimer should return error on terminated loop")
	}
	if unrefErr != ErrLoopTerminated {
		t.Fatalf("UnrefTimer should return ErrLoopTerminated, got %v", unrefErr)
	}
}
