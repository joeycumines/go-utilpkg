package eventloop

// ============================================================================
// Autopsy Kill Conditions — Node.js v11+ Microtask/nextTick Ordering
//
// These tests verify that the event loop correctly implements Node.js v11+
// semantics where microtasks (promises, queueMicrotask) and nextTick callbacks
// are drained between phases and per-task (for internal queue tasks),
// regardless of the strictMicrotaskOrdering option.
//
// Key fixes being verified:
//   - Task 5: processInternalQueue drains microtasks per-task (unconditional)
//   - Task 6: Inter-phase drains in tick() between runTimers, processInternalQueue,
//     processExternal, and drainAuxJobs (unconditional)
//   - Task 7: Exhaustive draining with no budget cap (unconditional)
//
// Note: Per-callback draining in runTimers, processExternal, drainAuxJobs,
// and runAux auxJobs is still gated by strictMicrotaskOrdering (default false).
// KILL-001 uses WithStrictMicrotaskOrdering(true) for this reason.
// ============================================================================

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestKill001_PerTimerCallbackMicrotaskDraining verifies that microtasks
// queued during a timer callback drain before the NEXT timer callback fires
// in the same runTimers() phase.
//
// In Node.js v11+, each timer callback is followed by a microtask checkpoint.
// The eventloop's runTimers() still gates per-callback draining on
// strictMicrotaskOrdering, so this test uses WithStrictMicrotaskOrdering(true).
// Once the option is removed (or made default), this test will pass with New().
func TestKill001_PerTimerCallbackMicrotaskDraining(t *testing.T) {
	loop, err := New(WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("New(WithStrictMicrotaskOrdering(true)) failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var order []string
	var mu sync.Mutex
	done := make(chan struct{})

	appendOrder := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	// Submit all setup atomically within the loop goroutine to avoid cross-tick races.
	loop.Submit(func() {
		// Timer 1: queues a microtask during its callback.
		js.SetTimeout(func() {
			appendOrder("timer-1")
			js.QueueMicrotask(func() {
				appendOrder("microtask-from-timer-1")
			})
		}, 0)

		// Timer 2: should run AFTER the microtask from timer-1.
		js.SetTimeout(func() {
			appendOrder("timer-2")
			close(done)
		}, 0)
	})

	go func() {
		if err := loop.Run(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()
	waitForRunning(t, loop)

	defer func() {
		cancel()
		loop.Shutdown(context.Background())
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		mu.Lock()
		t.Fatalf("Timeout. Order: %v", order)
	}

	mu.Lock()
	defer mu.Unlock()

	// Expected: timer-1 -> microtask-from-timer-1 -> timer-2
	expected := []string{"timer-1", "microtask-from-timer-1", "timer-2"}
	if len(order) != len(expected) {
		t.Fatalf("Expected %d events, got %d: %v", len(expected), len(order), order)
	}
	for i, ev := range expected {
		if order[i] != ev {
			t.Errorf("order[%d]: expected %q, got %q", i, ev, order[i])
		}
	}
}

// TestKill002_PerInternalTaskMicrotaskDraining verifies that microtasks
// queued during an internal task (SubmitInternal) drain before the NEXT
// internal task runs. This is fixed unconditionally by Task 5
// (processInternalQueue drains per-task without guard).
//
// It includes both fast path and poll path subtests.
func TestKill002_PerInternalTaskMicrotaskDraining(t *testing.T) {
	// setupAndRun creates a loop with the given fast path mode, schedules two
	// internal tasks where the first queues a microtask, and verifies the
	// microtask runs between the two tasks.
	setupAndRun := func(t *testing.T, useFastPath bool) {
		t.Helper()

		loop, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		if !useFastPath {
			if err := loop.SetFastPathMode(FastPathDisabled); err != nil {
				t.Fatalf("SetFastPathMode failed: %v", err)
			}
		}

		js, err := NewJS(loop)
		if err != nil {
			t.Fatalf("NewJS() failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var order []string
		var mu sync.Mutex
		done := make(chan struct{})

		appendOrder := func(s string) {
			mu.Lock()
			order = append(order, s)
			mu.Unlock()
		}

		pathName := "poll-path"
		if useFastPath {
			pathName = "fast-path"
		}

		// Internal task 1: queues a microtask.
		task1 := func() {
			appendOrder("internal-task-1-" + pathName)
			js.QueueMicrotask(func() {
				appendOrder("microtask-from-task-1-" + pathName)
			})
		}

		// Internal task 2: should run AFTER the microtask from task 1.
		task2 := func() {
			appendOrder("internal-task-2-" + pathName)
			close(done)
		}

		go func() {
			if err := loop.Run(ctx); err != nil && ctx.Err() == nil {
				t.Errorf("loop.Run failed: %v", err)
			}
		}()
		// With FastPathDisabled, the loop quickly transitions to StateSleeping
		// in poll(). waitForRunning (StateRunning only) would miss it.
		// waitLoopState for StateSleeping catches the poll path directly.
		// For fast path, the loop stays in StateRunning (no poll), so
		// waitForRunning is used instead.
		if useFastPath {
			waitForRunning(t, loop)
		} else {
			waitLoopState(t, loop, StateSleeping, 5*time.Second)
		}

		defer loop.Shutdown(context.Background())

		// Submit task 1 first, then task 2.
		if err := loop.SubmitInternal(task1); err != nil {
			t.Fatalf("SubmitInternal(task1) failed: %v", err)
		}
		if err := loop.SubmitInternal(task2); err != nil {
			t.Fatalf("SubmitInternal(task2) failed: %v", err)
		}

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			mu.Lock()
			t.Fatalf("Timeout. Order: %v", order)
		}

		mu.Lock()
		defer mu.Unlock()

		expected := []string{
			"internal-task-1-" + pathName,
			"microtask-from-task-1-" + pathName,
			"internal-task-2-" + pathName,
		}
		if len(order) != len(expected) {
			t.Fatalf("Expected %d events, got %d: %v", len(expected), len(order), order)
		}
		for i, ev := range expected {
			if order[i] != ev {
				t.Errorf("order[%d]: expected %q, got %q", i, ev, order[i])
			}
		}
	}

	t.Run("FastPath", func(t *testing.T) {
		setupAndRun(t, true)
	})
	t.Run("PollPath", func(t *testing.T) {
		setupAndRun(t, false)
	})
}

// TestKill003_MicrotaskBudgetOverflow verifies that a large batch of
// microtasks (exceeding the old 1024 budget) all drain exhaustively.
// This is fixed unconditionally by Task 7 (exhaustive draining, no budget).
func TestKill003_MicrotaskBudgetOverflow(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const numMicrotasks = 1500 // exceeds old budget of 1024
	var count atomic.Int64
	done := make(chan struct{})

	// Submit all microtasks from within the loop goroutine to ensure
	// they are queued in the same tick before draining begins.
	loop.Submit(func() {
		for range numMicrotasks {
			js.QueueMicrotask(func() {
				if count.Add(1) == int64(numMicrotasks) {
					close(done)
				}
			})
		}
	})

	go func() {
		if err := loop.Run(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()
	waitForRunning(t, loop)

	defer func() {
		cancel()
		loop.Shutdown(context.Background())
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("Timeout: only %d/%d microtasks drained", count.Load(), numMicrotasks)
	}

	if got := count.Load(); got != int64(numMicrotasks) {
		t.Errorf("Expected %d microtasks, got %d", numMicrotasks, got)
	}
}

// TestKill004_InterPhaseMicrotaskLeakage verifies that a microtask queued
// during a timer callback drains in the inter-phase drain (between runTimers
// and processInternalQueue), BEFORE any internal task runs.
// This is fixed unconditionally by Task 6 (inter-phase drains in tick()).
func TestKill004_InterPhaseMicrotaskLeakage(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	// Force poll path (tick()) so that runTimers() -> inter-phase drain ->
	// processInternalQueue() ordering is exercised. In fast path, runAux()
	// drains the internal queue before tick() runs runTimers().
	if err := loop.SetFastPathMode(FastPathDisabled); err != nil {
		t.Fatalf("SetFastPathMode failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var order []string
	var mu sync.Mutex
	done := make(chan struct{})

	appendOrder := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	go func() {
		if err := loop.Run(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()
	// With FastPathDisabled, the loop quickly transitions to StateSleeping
	// in poll(). Wait for that state so we know the loop is ready.
	waitLoopState(t, loop, StateSleeping, 5*time.Second)

	defer func() {
		cancel()
		loop.Shutdown(context.Background())
	}()

	// Submit all setup atomically within the loop goroutine.
	// With FastPathDisabled, SubmitInternal does NOT execute inline even on
	// the loop thread (canUseFastPath() returns false), so the internal task
	// lands in the internal queue for the next tick().
	loop.Submit(func() {
		// Timer: fires in runTimers(), queues a microtask.
		js.SetTimeout(func() {
			appendOrder("timer")
			js.QueueMicrotask(func() {
				appendOrder("microtask-from-timer")
			})
		}, 0)

		// Internal task: lands in the internal queue. The inter-phase drain
		// (Task 6) between runTimers() and processInternalQueue() must drain
		// the microtask before this task runs.
		loop.SubmitInternal(func() {
			appendOrder("internal-task")
			close(done)
		})
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		mu.Lock()
		t.Fatalf("Timeout. Order: %v", order)
	}

	mu.Lock()
	defer mu.Unlock()

	// Expected: timer -> microtask-from-timer -> internal-task
	// The microtask drains in the inter-phase drain before the internal task.
	expected := []string{"timer", "microtask-from-timer", "internal-task"}
	if len(order) != len(expected) {
		t.Fatalf("Expected %d events, got %d: %v", len(expected), len(order), order)
	}
	for i, ev := range expected {
		if order[i] != ev {
			t.Errorf("order[%d]: expected %q, got %q", i, ev, order[i])
		}
	}
}

// TestKill005_NextTickStarvationViaBudget verifies that a large batch of
// nextTick callbacks (exceeding any old budget) all drain exhaustively,
// and that a microtask queued alongside them also runs.
// This is fixed unconditionally by Task 7 (exhaustive draining, no budget).
func TestKill005_NextTickStarvationViaBudget(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const numNextTicks = 1500 // exceeds old budget of 1024
	var nextTickCount atomic.Int64
	var microtaskRan atomic.Bool
	done := make(chan struct{})

	// Submit all nextTick callbacks and a microtask from within the loop goroutine.
	loop.Submit(func() {
		for range numNextTicks {
			loop.ScheduleNextTick(func() {
				if nextTickCount.Add(1) == int64(numNextTicks) {
					if microtaskRan.Load() {
						close(done)
					}
				}
			})
		}

		// Queue a microtask AFTER all nextTicks. In Node.js, nextTicks have
		// higher priority than microtasks, so all nextTicks drain first,
		// then the microtask runs. The key assertion is that NO nextTick is
		// starved by a budget cap.
		js.QueueMicrotask(func() {
			microtaskRan.Store(true)
			// If all nextTicks have already completed, signal done.
			if nextTickCount.Load() == int64(numNextTicks) {
				close(done)
			}
		})
	})

	go func() {
		if err := loop.Run(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()
	waitForRunning(t, loop)

	defer func() {
		cancel()
		loop.Shutdown(context.Background())
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("Timeout: nextTick=%d/%d microtaskRan=%v",
			nextTickCount.Load(), numNextTicks, microtaskRan.Load())
	}

	if got := nextTickCount.Load(); got != int64(numNextTicks) {
		t.Errorf("Expected %d nextTicks, got %d", numNextTicks, got)
	}
	if !microtaskRan.Load() {
		t.Error("Microtask did not run after nextTicks drained")
	}
}

// TestTransition_NextTickBeforeMicrotaskWithinDrain verifies that within a
// single drain cycle, nextTick callbacks run before regular microtasks
// (promise reactions, queueMicrotask). This matches Node.js semantics where
// process.nextTick has higher priority than Promise microtasks.
//
// The test uses New() (no options) — the fix is unconditional.
func TestTransition_NextTickBeforeMicrotaskWithinDrain(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var order []string
	var mu sync.Mutex
	done := make(chan struct{})

	appendOrder := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	// Submit all setup from within the loop goroutine to ensure
	// nextTick and microtask are queued in the same tick.
	loop.Submit(func() {
		// Queue a microtask FIRST (lower priority).
		js.QueueMicrotask(func() {
			appendOrder("microtask")
			close(done)
		})

		// Queue a nextTick SECOND (higher priority — should run first).
		loop.ScheduleNextTick(func() {
			appendOrder("nextTick")
		})
	})

	go func() {
		if err := loop.Run(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()
	waitForRunning(t, loop)

	defer func() {
		cancel()
		loop.Shutdown(context.Background())
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		mu.Lock()
		t.Fatalf("Timeout. Order: %v", order)
	}

	mu.Lock()
	defer mu.Unlock()

	// Expected: nextTick runs before microtask, despite being queued after.
	// nextTick has higher priority in the drain loop.
	expected := []string{"nextTick", "microtask"}
	if len(order) != len(expected) {
		t.Fatalf("Expected %d events, got %d: %v", len(expected), len(order), order)
	}
	for i, ev := range expected {
		if order[i] != ev {
			t.Errorf("order[%d]: expected %q, got %q", i, ev, order[i])
		}
	}
}

// TestTransition_NestedNextTickRecursion verifies that nextTick callbacks
// that recursively schedule more nextTicks (and microtasks) all drain
// exhaustively in a single drain cycle, with nextTicks always running
// before microtasks at each level.
//
// The test uses New() (no options) — the fix is unconditional.
func TestTransition_NestedNextTickRecursion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var order []string
	var mu sync.Mutex
	done := make(chan struct{})

	appendOrder := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	const depth = 5

	// recursiveNextTick schedules a nextTick that, when run, queues another
	// nextTick (depth-1) and a microtask. The nextTick should always run
	// before the microtask at each level.
	var recursiveNextTick func(level int)
	recursiveNextTick = func(level int) {
		if level <= 0 {
			appendOrder("nextTick-base")
			// Queue a final microtask to verify it runs after all nextTicks.
			js.QueueMicrotask(func() {
				appendOrder("microtask-final")
				close(done)
			})
			return
		}

		// Queue a microtask at this level — it should run AFTER the
		// nextTick at the next (deeper) level, because nextTicks drain first.
		js.QueueMicrotask(func() {
			appendOrder("microtask-level-" + itoa(level))
		})

		// Queue nextTick for the next level — higher priority.
		loop.ScheduleNextTick(func() {
			appendOrder("nextTick-level-" + itoa(level))
			recursiveNextTick(level - 1)
		})
	}

	// Submit all setup from within the loop goroutine.
	loop.Submit(func() {
		recursiveNextTick(depth)
	})

	go func() {
		if err := loop.Run(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()
	waitForRunning(t, loop)

	defer func() {
		cancel()
		loop.Shutdown(context.Background())
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		mu.Lock()
		t.Fatalf("Timeout. Order: %v", order)
	}

	mu.Lock()
	defer mu.Unlock()

	// Build the expected order:
	// At each level, nextTick runs first (higher priority), then recursion
	// queues more nextTicks and microtasks. All nextTicks at deeper levels
	// run before microtasks at shallower levels.
	//
	// Level 5: nextTick-level-5 -> (queues nextTick-4 + microtask-5)
	// Level 4: nextTick-level-4 -> (queues nextTick-3 + microtask-4)
	// Level 3: nextTick-level-3 -> (queues nextTick-2 + microtask-3)
	// Level 2: nextTick-level-2 -> (queues nextTick-1 + microtask-2)
	// Level 1: nextTick-level-1 -> (queues nextTick-base + microtask-1)
	// Base:    nextTick-base -> (queues microtask-final)
	//
	// Then all microtasks run in FIFO order:
	// microtask-level-5, microtask-level-4, ..., microtask-level-1, microtask-final
	expected := []string{
		"nextTick-level-5",
		"nextTick-level-4",
		"nextTick-level-3",
		"nextTick-level-2",
		"nextTick-level-1",
		"nextTick-base",
		"microtask-level-5",
		"microtask-level-4",
		"microtask-level-3",
		"microtask-level-2",
		"microtask-level-1",
		"microtask-final",
	}

	if len(order) != len(expected) {
		t.Fatalf("Expected %d events, got %d: %v", len(expected), len(order), order)
	}
	for i, ev := range expected {
		if order[i] != ev {
			t.Errorf("order[%d]: expected %q, got %q", i, ev, order[i])
		}
	}
}

// itoa converts an integer to its decimal string representation.
// This avoids importing strconv for a single use.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
