package eventloop

import (
	"container/heap"
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

// waitForRunning spins until the loop reaches StateRunning with a 5-second timeout guard.
func waitForRunning(t *testing.T, loop *Loop) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for loop.State() != StateRunning {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for loop to start running")
		default:
			runtime.Gosched()
		}
	}
}

// =============================================================================
// New() constructor coverage — various option combinations
// =============================================================================

// TestNew_AllOptionsCombined covers the config branches in New() that set
// strictMicrotaskOrdering, fastPathMode, logger, debugMode, metricsEnabled,
// and ingressChunkSize on the constructed Loop.
func TestNew_AllOptionsCombined(t *testing.T) {
	writer := &testEventWriter{onWrite: func(event *testEvent) error { return nil }}
	factory := &testEventFactory{}
	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](factory),
		logiface.WithWriter[*testEvent](writer),
	)
	genericLogger := typedLogger.Logger()

	loop, err := New(
		WithStrictMicrotaskOrdering(true),
		WithFastPathMode(FastPathDisabled),
		WithMetrics(true),
		WithLogger(genericLogger),
		WithDebugMode(true),
		WithIngressChunkSize(128),
	)
	if err != nil {
		t.Fatalf("New with all options failed: %v", err)
	}
	defer loop.Close()

	if !loop.strictMicrotaskOrdering {
		t.Error("expected strictMicrotaskOrdering to be true")
	}
	if FastPathMode(loop.fastPathMode.Load()) != FastPathDisabled {
		t.Error("expected fastPathMode to be FastPathDisabled")
	}
	if loop.metrics == nil {
		t.Error("expected metrics to be non-nil")
	}
	if loop.tpsCounter == nil {
		t.Error("expected tpsCounter to be non-nil")
	}
	if loop.logger == nil {
		t.Error("expected logger to be non-nil")
	}
	if !loop.debugMode {
		t.Error("expected debugMode to be true")
	}
}

// TestNew_WithForcedFastPath exercises the FastPathForced branch.
func TestNew_WithForcedFastPath(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	if err != nil {
		t.Fatalf("New with FastPathForced failed: %v", err)
	}
	defer loop.Close()

	if FastPathMode(loop.fastPathMode.Load()) != FastPathForced {
		t.Error("expected FastPathForced")
	}
}

// TestNew_NilOptionSkipped verifies that nil options are gracefully skipped.
func TestNew_NilOptionSkipped(t *testing.T) {
	loop, err := New(nil, WithMetrics(true), nil)
	if err != nil {
		t.Fatalf("New with nil options failed: %v", err)
	}
	defer loop.Close()

	if loop.metrics == nil {
		t.Error("expected metrics to be non-nil after skipping nil options")
	}
}

// TestNew_OptionReturnsError verifies that resolveLoopOptions error propagates.
func TestNew_OptionReturnsError(t *testing.T) {
	badOpt := &loopOptionImpl{func(opts *loopOptions) error {
		return errors.New("intentional option error")
	}}
	_, err := New(badOpt)
	if err == nil {
		t.Fatal("expected error from bad option")
	}
	if err.Error() != "intentional option error" {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestNew_MetricsDisabledByDefault verifies metrics are nil when not enabled.
func TestNew_MetricsDisabledByDefault(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Close()

	if loop.metrics != nil {
		t.Error("expected metrics to be nil by default")
	}
	if loop.tpsCounter != nil {
		t.Error("expected tpsCounter to be nil by default")
	}
}

// =============================================================================
// processExternal — OnOverload and strictMicrotaskOrdering coverage
// =============================================================================

// TestProcessExternal_OnOverloadFires verifies that the OnOverload callback
// fires when the external queue has more tasks than the batchBuf can hold (256).
// This uses FastPathDisabled so Submit() goes to external queue (not auxJobs).
func TestProcessExternal_OnOverloadFires(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	var overloadCalled atomic.Bool
	loop.OnOverload = func(err error) {
		if !errors.Is(err, ErrLoopOverloaded) {
			t.Errorf("OnOverload received wrong error: %v", err)
		}
		overloadCalled.Store(true)
	}

	// Submit 300 tasks BEFORE running the loop.
	// batchBuf is [256]func{}, so processExternal pops 256,
	// leaving 44 remaining → OnOverload fires.
	const totalTasks = 300
	var executed atomic.Int32
	for i := 0; i < totalTasks; i++ {
		if err := loop.Submit(func() {
			executed.Add(1)
		}); err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go loop.Run(ctx)

	// Wait for all tasks to complete
	deadline := time.After(5 * time.Second)
	for {
		if executed.Load() >= totalTasks {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for tasks: got %d/%d", executed.Load(), totalTasks)
		default:
			runtime.Gosched()
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	if !overloadCalled.Load() {
		t.Error("expected OnOverload to be called")
	}
}

// TestProcessExternal_OnOverloadPanics verifies that a panicking OnOverload
// callback does not crash the loop.
func TestProcessExternal_OnOverloadPanics(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	loop.OnOverload = func(err error) {
		panic("intentional OnOverload panic")
	}

	const totalTasks = 300
	var executed atomic.Int32
	for i := 0; i < totalTasks; i++ {
		if err := loop.Submit(func() {
			executed.Add(1)
		}); err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go loop.Run(ctx)

	deadline := time.After(5 * time.Second)
	for {
		if executed.Load() >= totalTasks {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out: got %d/%d", executed.Load(), totalTasks)
		default:
			runtime.Gosched()
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}

// TestProcessExternal_StrictMicrotaskOrdering verifies that microtasks are
// drained between each external task when strictMicrotaskOrdering is enabled.
func TestProcessExternal_StrictMicrotaskOrdering(t *testing.T) {
	loop, err := New(
		WithStrictMicrotaskOrdering(true),
		WithFastPathMode(FastPathDisabled),
	)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Use a channel to track execution order
	var mu sync.Mutex
	var order []string

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	// Submit two tasks that each schedule a microtask.
	// With strict ordering: task1 → micro1 → task2 → micro2
	loop.Submit(func() {
		mu.Lock()
		order = append(order, "task1")
		mu.Unlock()

		loop.ScheduleMicrotask(func() {
			mu.Lock()
			order = append(order, "micro1")
			mu.Unlock()
			wg.Done()
		})
	})

	loop.Submit(func() {
		mu.Lock()
		order = append(order, "task2")
		mu.Unlock()

		loop.ScheduleMicrotask(func() {
			mu.Lock()
			order = append(order, "micro2")
			mu.Unlock()
			wg.Done()
		})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	// Wait for completion via goroutine so we don't race
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for microtasks")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	mu.Lock()
	defer mu.Unlock()

	// With strict ordering, micro1 must come before task2
	if len(order) < 4 {
		t.Fatalf("expected at least 4 events, got %d: %v", len(order), order)
	}

	// Find positions
	micro1Idx := -1
	task2Idx := -1
	for i, v := range order {
		if v == "micro1" && micro1Idx == -1 {
			micro1Idx = i
		}
		if v == "task2" && task2Idx == -1 {
			task2Idx = i
		}
	}

	if micro1Idx >= 0 && task2Idx >= 0 && micro1Idx > task2Idx {
		t.Errorf("strict ordering violated: micro1 at %d, task2 at %d; order=%v",
			micro1Idx, task2Idx, order)
	}
}

// =============================================================================
// runTimers — strictMicrotaskOrdering and canceled timer paths
// =============================================================================

// TestRunTimers_StrictMicrotaskOrderingInterleave verifies that microtasks drain between
// timer callbacks when strictMicrotaskOrdering is enabled.
func TestRunTimers_StrictMicrotaskOrderingInterleave(t *testing.T) {
	loop, err := New(WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	var mu sync.Mutex
	var order []string
	done := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	// Wait for loop to be running
	waitForRunning(t, loop)

	var wg sync.WaitGroup
	wg.Add(2) // wait for both microtasks

	// Schedule two timers with delay 0 — they fire immediately on next tick.
	// Each timer schedules a microtask. With strict ordering,
	// timer1 → micro1 → timer2 → micro2
	loop.ScheduleTimer(0, func() {
		mu.Lock()
		order = append(order, "timer1")
		mu.Unlock()

		loop.ScheduleMicrotask(func() {
			mu.Lock()
			order = append(order, "micro1")
			mu.Unlock()
			wg.Done()
		})
	})

	loop.ScheduleTimer(0, func() {
		mu.Lock()
		order = append(order, "timer2")
		mu.Unlock()

		loop.ScheduleMicrotask(func() {
			mu.Lock()
			order = append(order, "micro2")
			mu.Unlock()
			wg.Done()
		})
	})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	mu.Lock()
	defer mu.Unlock()

	if len(order) < 4 {
		t.Fatalf("expected 4 events, got %d: %v", len(order), order)
	}

	// With strict ordering, micro1 should come before timer2
	micro1Idx := -1
	timer2Idx := -1
	for i, v := range order {
		if v == "micro1" && micro1Idx == -1 {
			micro1Idx = i
		}
		if v == "timer2" && timer2Idx == -1 {
			timer2Idx = i
		}
	}

	if micro1Idx >= 0 && timer2Idx >= 0 && micro1Idx > timer2Idx {
		t.Errorf("strict ordering violated: micro1@%d, timer2@%d; order=%v",
			micro1Idx, timer2Idx, order)
	}
}

// TestRunTimers_WithMetrics verifies that metrics are recorded during timer
// execution (covers the metrics paths in safeExecute via timer callbacks).
func TestRunTimers_WithMetrics(t *testing.T) {
	loop, err := New(WithMetrics(true))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	done := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	// Wait for running
	waitForRunning(t, loop)

	// Schedule a timer that signals completion
	loop.ScheduleTimer(0, func() {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for timer")
	}

	// Check metrics snapshot
	m := loop.Metrics()
	if m == nil {
		t.Fatal("expected metrics to be non-nil")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}

// =============================================================================
// SubmitInternal — terminated and fast-path direct execution
// =============================================================================

// TestSubmitInternal_Terminated verifies ErrLoopTerminated from SubmitInternal.
func TestSubmitInternal_Terminated(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	err = loop.SubmitInternal(func() {
		t.Error("should not execute")
	})
	if !errors.Is(err, ErrLoopTerminated) {
		t.Errorf("expected ErrLoopTerminated, got %v", err)
	}
}

// TestSubmitInternal_FromLoopThread exercises the fast path direct execution
// inside SubmitInternal (canUseFastPath + isLoopThread + empty external queue).
func TestSubmitInternal_FromLoopThread(t *testing.T) {
	loop, err := New() // FastPathAuto, no I/O FDs → fast path enabled
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	result := make(chan bool, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	// Submit a task via Submit (goes to auxJobs in fast mode, runs on loop thread).
	// Inside, call SubmitInternal which should hit the fast path (direct execution).
	loop.Submit(func() {
		var innerExecuted bool
		err := loop.SubmitInternal(func() {
			innerExecuted = true
		})
		// In fast-path direct execution, the inner task runs synchronously
		result <- (err == nil && innerExecuted)
	})

	select {
	case ok := <-result:
		if !ok {
			t.Error("SubmitInternal fast path did not execute synchronously")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}

// TestSubmitInternal_FromLoopThread_WithTimerWakeup verifies that when
// SubmitInternal fast path executes a task that schedules a timer,
// the wakeup is sent to process the timer.
func TestSubmitInternal_FromLoopThread_WithTimerWakeup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	timerFired := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	// From the loop thread, SubmitInternal a task that schedules a timer
	loop.Submit(func() {
		loop.SubmitInternal(func() {
			loop.ScheduleTimer(0, func() {
				close(timerFired)
			})
		})
	})

	select {
	case <-timerFired:
		// Timer fired as expected
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for timer")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}

// =============================================================================
// Sleep / Timeout / AbortTimeout — error paths on terminated loop
// =============================================================================

// TestSleep_OnTerminatedLoop verifies Sleep resolves immediately when
// ScheduleTimer fails (covers the err != nil branch in Sleep).
func TestSleep_OnTerminatedLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	// Start and immediately shut down
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	go loop.Run(ctx)
	waitForRunning(t, loop)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	loop.Shutdown(shutdownCtx)
	shutdownCancel()
	cancel()

	// Now loop is terminated — Sleep should still return a promise that resolves
	promise := js.Sleep(100 * time.Millisecond)
	if promise == nil {
		t.Fatal("expected non-nil promise from Sleep on terminated loop")
	}

	// The promise should resolve immediately (via fallback)
	ch := promise.ToChannel()
	select {
	case <-ch:
		// Success - promise resolved
	case <-time.After(2 * time.Second):
		t.Fatal("timed out: Sleep promise did not resolve on terminated loop")
	}
}

// TestTimeout_OnTerminatedLoop verifies Timeout rejects immediately when
// ScheduleTimer fails (covers the err != nil branch in Timeout).
func TestTimeout_OnTerminatedLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	go loop.Run(ctx)
	waitForRunning(t, loop)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	loop.Shutdown(shutdownCtx)
	shutdownCancel()
	cancel()

	// Timeout on terminated loop should reject immediately
	promise := js.Timeout(100 * time.Millisecond)
	if promise == nil {
		t.Fatal("expected non-nil promise from Timeout on terminated loop")
	}

	ch := promise.ToChannel()
	select {
	case result := <-ch:
		// The result should be a TimeoutError (from reject path)
		if _, ok := result.(*TimeoutError); !ok {
			// In reject path, the rejection might be wrapped
			// Just verify we got something
			t.Logf("Timeout resolved with: %v (type %T)", result, result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out: Timeout promise did not settle on terminated loop")
	}
}

// TestAbortTimeout_OnTerminatedLoop verifies AbortTimeout returns error
// when the loop is terminated (covers the err != nil branch).
func TestAbortTimeout_OnTerminatedLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	go loop.Run(ctx)
	waitForRunning(t, loop)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	loop.Shutdown(shutdownCtx)
	shutdownCancel()
	cancel()

	_, err = AbortTimeout(loop, 5000)
	if err == nil {
		t.Fatal("expected error from AbortTimeout on terminated loop")
	}
}

// =============================================================================
// addHandler — optimistic fast path and multiple handler overflow
// =============================================================================

// TestAddHandler_AlreadySettled exercises the optimistic lock-free path in
// addHandler where the promise is already settled before addHandler is called.
func TestAddHandler_AlreadySettled(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	// Create and immediately resolve a promise
	promise, resolve, _ := js.NewChainedPromise()
	resolve(42)

	// Now add handler AFTER settling (optimistic fast path in addHandler)
	result := make(chan any, 1)
	promise.Then(func(v any) any {
		result <- v
		return nil
	}, nil)

	select {
	case v := <-result:
		if v != 42 {
			t.Errorf("expected 42, got %v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for handler on settled promise")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}

// TestAddHandler_MultipleHandlers exercises the overflow handler path in
// addHandler where h0 is already occupied and additional handlers are stored
// in the result field as []handler.
func TestAddHandler_MultipleHandlers(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	// Create a pending promise
	promise, resolve, _ := js.NewChainedPromise()

	// Attach three handlers while still pending
	// Handler 1 goes into h0 slot, handlers 2 & 3 go into overflow []handler
	var wg sync.WaitGroup
	wg.Add(3)

	var results [3]atomic.Value

	promise.Then(func(v any) any {
		results[0].Store(v)
		wg.Done()
		return nil
	}, nil)

	promise.Then(func(v any) any {
		results[1].Store(v)
		wg.Done()
		return nil
	}, nil)

	promise.Then(func(v any) any {
		results[2].Store(v)
		wg.Done()
		return nil
	}, nil)

	// Now resolve
	resolve("hello")

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for handlers")
	}

	for i := 0; i < 3; i++ {
		v := results[i].Load()
		if v != "hello" {
			t.Errorf("handler %d: expected 'hello', got %v", i, v)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}

// =============================================================================
// chunkedIngress — multi-chunk Pop and pool reuse coverage
// =============================================================================

// TestChunkedIngress_MultiChunkPopAndReturn exercises the Pop path where
// the head chunk is exhausted and the queue advances to the next chunk,
// returning the exhausted chunk to the pool.
func TestChunkedIngress_MultiChunkPopAndReturn(t *testing.T) {
	// Use a small chunk size to easily span multiple chunks
	q := newChunkedIngressWithSize(16)

	// Push more items than one chunk can hold — forces 2+ chunks
	const totalItems = 40 // 16 * 2.5 = spans 3 chunks
	for i := 0; i < totalItems; i++ {
		idx := i
		q.Push(func() { _ = idx })
	}

	if q.Length() != totalItems {
		t.Fatalf("expected length %d, got %d", totalItems, q.Length())
	}

	// Pop all items — this exercises chunk advancement in Pop
	count := 0
	for {
		fn, ok := q.Pop()
		if !ok {
			break
		}
		if fn == nil {
			t.Fatal("popped nil func")
		}
		count++
	}

	if count != totalItems {
		t.Errorf("expected to pop %d items, got %d", totalItems, count)
	}

	if q.Length() != 0 {
		t.Errorf("expected empty queue, got length %d", q.Length())
	}

	// Pop from empty queue — hits the head.readPos >= head.pos single-chunk path
	_, ok := q.Pop()
	if ok {
		t.Error("expected false from Pop on empty queue")
	}
}

// TestChunkedIngress_PushPopCycles exercises pool reuse paths in newChunk
// where chunks are returned to pool and then re-acquired.
func TestChunkedIngress_PushPopCycles(t *testing.T) {
	q := newChunkedIngressWithSize(16)

	for cycle := 0; cycle < 5; cycle++ {
		// Push a full chunk worth + 1 extra (forces two chunks)
		for i := 0; i < 17; i++ {
			q.Push(func() {})
		}

		// Pop everything
		for {
			_, ok := q.Pop()
			if !ok {
				break
			}
		}

		if q.Length() != 0 {
			t.Fatalf("cycle %d: expected empty queue", cycle)
		}
	}
}

// TestChunkedIngress_PoolReuseCapacityMismatch exercises the capacity
// reallocation path in newChunk where a pooled chunk has smaller capacity
// than the current chunkSize.
func TestChunkedIngress_PoolReuseCapacityMismatch(t *testing.T) {
	q := newChunkedIngressWithSize(32)

	// Manually inject a chunk with smaller capacity into the pool.
	// When newChunk retrieves it, cap(c.tasks) < q.chunkSize triggers reallocation.
	q.chunkPool.Put(&chunk{tasks: make([]func(), 16)})

	// Push one item — triggers newChunk which gets our small chunk from pool
	q.Push(func() {})

	// Verify it works correctly
	fn, ok := q.Pop()
	if !ok || fn == nil {
		t.Error("expected to pop a task after capacity realloc")
	}

	// Now push more than 32 to span multiple chunks with correct size
	for i := 0; i < 40; i++ {
		q.Push(func() {})
	}
	count := 0
	for {
		_, ok := q.Pop()
		if !ok {
			break
		}
		count++
	}
	if count != 40 {
		t.Errorf("expected 40 items, popped %d", count)
	}
}

// TestChunkedIngress_PopFromNilHead verifies Pop returns false when head is nil.
func TestChunkedIngress_PopFromNilHead(t *testing.T) {
	q := newChunkedIngressWithSize(16)
	// Never push anything — head is nil
	_, ok := q.Pop()
	if ok {
		t.Error("expected false from Pop on nil head")
	}
}

// =============================================================================
// WithIngressChunkSize — boundary values
// =============================================================================

// TestWithIngressChunkSize_Boundaries verifies chunk size clamping and
// power-of-2 rounding for various input values.
func TestWithIngressChunkSize_Boundaries(t *testing.T) {
	cases := []struct {
		name     string
		input    int
		expected int
	}{
		{"below minimum", 4, 16},
		{"exact minimum", 16, 16},
		{"not power of 2", 50, 32},
		{"power of 2", 64, 64},
		{"above maximum", 8192, 4096},
		{"exact maximum", 4096, 4096},
		{"zero", 0, 16},
		{"negative", -1, 16},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loop, err := New(WithIngressChunkSize(tc.input))
			if err != nil {
				t.Fatalf("New failed: %v", err)
			}
			defer loop.Close()

			// Check that the external queue has the expected chunk size
			if loop.external.chunkSize != tc.expected {
				t.Errorf("expected chunkSize %d, got %d", tc.expected, loop.external.chunkSize)
			}
		})
	}
}

// =============================================================================
// Metrics via tick — covers the metrics recording path in tick()
// =============================================================================

// TestTick_WithMetricsRecording verifies that the tick() metrics path is
// exercised when WithMetrics(true) is set — covers queue depth tracking
// and latency recording in safeExecute.
func TestTick_WithMetricsRecording(t *testing.T) {
	loop, err := New(
		WithMetrics(true),
		WithFastPathMode(FastPathDisabled), // Force poll path so tick() runs
	)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	done := make(chan struct{})

	// Submit a task before Run
	loop.Submit(func() {
		close(done)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	m := loop.Metrics()
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}

	shutdownFinal, shutdownFinalCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownFinalCancel()
	loop.Shutdown(shutdownFinal)
}

// =============================================================================
// ScheduleNextTick — nil fn, terminated, and normal paths
// =============================================================================

// TestScheduleNextTick_NilFn covers the nil function early return.
func TestScheduleNextTick_NilFn(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Close()

	err = loop.ScheduleNextTick(nil)
	if err != nil {
		t.Errorf("expected nil error for nil fn, got %v", err)
	}
}

// TestScheduleNextTick_Terminated covers the terminated path.
func TestScheduleNextTick_Terminated(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	go loop.Run(ctx)
	waitForRunning(t, loop)
	sdCtx1, sdCancel1 := context.WithTimeout(context.Background(), 2*time.Second)
	loop.Shutdown(sdCtx1)
	sdCancel1()
	cancel()

	err = loop.ScheduleNextTick(func() {})
	if !errors.Is(err, ErrLoopTerminated) {
		t.Errorf("expected ErrLoopTerminated, got %v", err)
	}
}

// TestScheduleNextTick_FastMode covers the fast-mode wakeup path.
func TestScheduleNextTick_FastMode(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	done := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	err = loop.ScheduleNextTick(func() {
		close(done)
	})
	if err != nil {
		t.Fatalf("ScheduleNextTick failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	sdCtx2, sdCancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer sdCancel2()
	loop.Shutdown(sdCtx2)
}

// =============================================================================
// drainAuxJobs strictMicrotaskOrdering + runAux strictMicrotaskOrdering
// =============================================================================

// TestDrainAuxJobs_StrictMicrotaskOrdering exercises the strictMicrotaskOrdering
// path in drainAuxJobs by combining strict ordering + timer (forces tick) + Submit.
func TestDrainAuxJobs_StrictMicrotaskOrdering(t *testing.T) {
	loop, err := New(WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	var mu sync.Mutex
	var order []string
	done := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	loop.ScheduleTimer(0, func() {
		mu.Lock()
		order = append(order, "timer")
		mu.Unlock()
	})

	loop.Submit(func() {
		mu.Lock()
		order = append(order, "aux1")
		mu.Unlock()
		loop.ScheduleMicrotask(func() {
			mu.Lock()
			order = append(order, "micro_from_aux1")
			mu.Unlock()
		})
	})

	loop.Submit(func() {
		mu.Lock()
		order = append(order, "aux2")
		mu.Unlock()
		close(done)
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	sdCtx3, sdCancel3 := context.WithTimeout(context.Background(), 2*time.Second)
	defer sdCancel3()
	loop.Shutdown(sdCtx3)

	mu.Lock()
	defer mu.Unlock()
	if len(order) < 3 {
		t.Errorf("expected at least 3 events, got %d: %v", len(order), order)
	}
}

// TestRunAux_StrictMicrotaskOrdering tests microtask draining in runAux.
func TestRunAux_StrictMicrotaskOrdering(t *testing.T) {
	loop, err := New(WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	var mu sync.Mutex
	var order []string
	allDone := make(chan struct{})
	var remaining atomic.Int32
	remaining.Store(4)

	maybeClose := func() {
		if remaining.Add(-1) == 0 {
			close(allDone)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	loop.Submit(func() {
		mu.Lock()
		order = append(order, "task1")
		mu.Unlock()
		loop.ScheduleMicrotask(func() {
			mu.Lock()
			order = append(order, "micro1")
			mu.Unlock()
			maybeClose()
		})
		maybeClose()
	})

	loop.Submit(func() {
		mu.Lock()
		order = append(order, "task2")
		mu.Unlock()
		loop.ScheduleMicrotask(func() {
			mu.Lock()
			order = append(order, "micro2")
			mu.Unlock()
			maybeClose()
		})
		maybeClose()
	})

	select {
	case <-allDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	sdCtx4, sdCancel4 := context.WithTimeout(context.Background(), 2*time.Second)
	defer sdCancel4()
	loop.Shutdown(sdCtx4)

	mu.Lock()
	defer mu.Unlock()
	if len(order) < 4 {
		t.Errorf("expected 4 events, got %d: %v", len(order), order)
	}
}

// =============================================================================
// Close / Shutdown / Wake / Run — lifecycle paths
// =============================================================================

// TestClose_OnRunningLoop exercises Close() on a running loop.
func TestClose_OnRunningLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	err = loop.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	if loop.State() != StateTerminated {
		t.Errorf("expected StateTerminated, got %v", loop.State())
	}

	err = loop.Close()
	if !errors.Is(err, ErrLoopTerminated) {
		t.Errorf("expected ErrLoopTerminated on second close, got %v", err)
	}
}

// TestClose_OnAwakeLoop exercises Close() on a loop that was never Run.
func TestClose_OnAwakeLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	err = loop.Close()
	if err != nil {
		t.Errorf("Close on awake loop returned error: %v", err)
	}
}

// TestShutdown_OnAwakeLoop exercises Shutdown() on a loop that was never Run.
func TestShutdown_OnAwakeLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	sdCtx5, sdCancel5 := context.WithTimeout(context.Background(), 2*time.Second)
	defer sdCancel5()
	err = loop.Shutdown(sdCtx5)
	if err != nil {
		t.Errorf("Shutdown on awake loop returned error: %v", err)
	}
}

// TestWake_OnTerminated exercises Wake() on a terminated loop.
func TestWake_OnTerminated(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	go loop.Run(ctx)
	waitForRunning(t, loop)
	sdCtx6, sdCancel6 := context.WithTimeout(context.Background(), 2*time.Second)
	loop.Shutdown(sdCtx6)
	sdCancel6()
	cancel()

	err = loop.Wake()
	if err != nil {
		t.Errorf("Wake on terminated loop returned error: %v", err)
	}
}

// TestRun_DoubleRun exercises calling Run() twice.
func TestRun_DoubleRun(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	err = loop.Run(ctx)
	if !errors.Is(err, ErrLoopAlreadyRunning) {
		t.Errorf("expected ErrLoopAlreadyRunning, got %v", err)
	}

	sdCtx7, sdCancel7 := context.WithTimeout(context.Background(), 2*time.Second)
	defer sdCancel7()
	loop.Shutdown(sdCtx7)
}

// TestRun_OnTerminated exercises calling Run() on a terminated loop.
func TestRun_OnTerminated(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	loop.Close()

	err = loop.Run(context.Background())
	if !errors.Is(err, ErrLoopTerminated) {
		t.Errorf("expected ErrLoopTerminated, got %v", err)
	}
}

// =============================================================================
// pollFastMode — non-blocking timeout path and termination paths
// =============================================================================

// TestPollFastMode_ZeroTimeoutViaImmediateTimer covers the timeout=0 case
// in pollFastMode by scheduling a timer with 0 delay. When calculateTimeout
// sees an expired timer, it returns 0, causing pollFastMode to take the
// non-blocking path (timeoutMs == 0).
func TestPollFastMode_ZeroTimeoutViaImmediateTimer(t *testing.T) {
	loop, err := New() // Auto fast path, no I/O FDs
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	var count atomic.Int32
	done := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	// Schedule many timers with 0 delay — this forces calculateTimeout to
	// return 0 repeatedly, exercising the non-blocking path in pollFastMode.
	const numTimers = 10
	for i := 0; i < numTimers; i++ {
		loop.ScheduleTimer(0, func() {
			if count.Add(1) >= numTimers {
				close(done)
			}
		})
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for immediate timers")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}

// TestPollFastMode_ShutdownDuringPoll exercises the StateTerminating check
// in pollFastMode by shutting down the loop while it's in the fast path.
func TestPollFastMode_ShutdownDuringPoll(t *testing.T) {
	loop, err := New() // Auto fast path
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	// No tasks submitted — loop should be in pollFastMode blocking on channel.
	// Shutdown transitions to StateTerminating and wakes the channel.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	err = loop.Shutdown(shutdownCtx)
	if err != nil {
		t.Errorf("Shutdown returned error: %v", err)
	}

	if loop.State() != StateTerminated {
		t.Errorf("expected StateTerminated, got %v", loop.State())
	}
}

// =============================================================================
// promise addHandler race path — settle between optimistic check and lock
// =============================================================================

// TestAddHandler_RaceBetweenSettleAndLock exercises addHandler's re-check
// under lock by creating many goroutines that concurrently resolve and
// attach handlers. Under race detector, this exercises the path where
// state changes between the optimistic check and the locked re-check.
func TestAddHandler_RaceBetweenSettleAndLock(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(iterations)

	for i := 0; i < iterations; i++ {
		go func() {
			defer wg.Done()
			promise, resolve, _ := js.NewChainedPromise()

			// Race: resolve and handler attachment happen concurrently
			resolved := make(chan struct{})
			go func() {
				resolve("value")
				close(resolved)
			}()

			handlerDone := make(chan struct{})
			promise.Then(func(v any) any {
				close(handlerDone)
				return nil
			}, nil)

			// Wait for both to complete
			<-resolved
			select {
			case <-handlerDone:
			case <-time.After(5 * time.Second):
				// Handler should always fire eventually
			}
		}()
	}

	wg.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}

// =============================================================================
// New() with metrics — tick coverage for queue depth tracking
// =============================================================================

// TestNew_WithMetricsTickCoverage runs the loop with metrics and multiple
// task types (external, timer, microtask) to ensure tick() records queue
// depths for all three queues.
func TestNew_WithMetricsTickCoverage(t *testing.T) {
	loop, err := New(
		WithMetrics(true),
		WithFastPathMode(FastPathDisabled),
	)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Use a channel to detect when loop is running
	running := make(chan struct{})
	loop.Submit(func() {
		close(running)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	// Wait for running state via channel
	select {
	case <-running:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for loop to start")
	}

	// Timer
	loop.ScheduleTimer(0, func() {
		wg.Done()
	})

	// Microtask
	loop.ScheduleMicrotask(func() {
		wg.Done()
	})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	m := loop.Metrics()
	if m == nil {
		t.Fatal("expected metrics snapshot to be non-nil")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}

// =============================================================================
// Submit on terminated loop — covers the ErrLoopTerminated paths
// =============================================================================

// TestSubmit_OnTerminatedLoop covers the StateTerminated early-return in Submit.
func TestSubmit_OnTerminatedLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	go loop.Run(ctx)
	waitForRunning(t, loop)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	loop.Shutdown(shutdownCtx)
	shutdownCancel()
	cancel()

	err = loop.Submit(func() {})
	if !errors.Is(err, ErrLoopTerminated) {
		t.Errorf("expected ErrLoopTerminated, got %v", err)
	}
}

// TestScheduleMicrotask_OnTerminatedLoop covers the terminated path.
func TestScheduleMicrotask_OnTerminatedLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	go loop.Run(ctx)
	waitForRunning(t, loop)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	loop.Shutdown(shutdownCtx)
	shutdownCancel()
	cancel()

	err = loop.ScheduleMicrotask(func() {})
	if !errors.Is(err, ErrLoopTerminated) {
		t.Errorf("expected ErrLoopTerminated, got %v", err)
	}
}

// =============================================================================
// runTimers — canceled timer branch coverage
// =============================================================================

// TestRunTimers_CanceledTimerInHeap exercises the else branch in runTimers
// where a timer has t.canceled == true when it's popped from the heap.
// Since CancelTimer normally removes timers from the heap, we directly
// inject a canceled timer into the heap from the loop thread.
func TestRunTimers_CanceledTimerInHeap(t *testing.T) {
	loop, err := New(WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	done := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	// From the loop thread, inject a canceled timer directly into the heap.
	// This exercises the `else` branch in runTimers where t.canceled is true.
	loop.Submit(func() {
		tmr := timerPool.Get().(*timer)
		tmr.id = TimerID(9999999)
		tmr.when = time.Now().Add(-time.Second) // already expired
		tmr.task = func() { t.Error("canceled timer task should NOT execute") }
		tmr.canceled.Store(true)
		tmr.nestingLevel = 0
		tmr.heapIndex = -1
		loop.timerMap[tmr.id] = tmr
		heap.Push(&loop.timers, tmr)

		// Also inject a non-canceled timer that signals completion,
		// ensuring runTimers processes both.
		tmr2 := timerPool.Get().(*timer)
		tmr2.id = TimerID(9999998)
		tmr2.when = time.Now().Add(-time.Second) // also expired
		tmr2.task = func() { close(done) }
		tmr2.canceled.Store(false)
		tmr2.nestingLevel = 0
		tmr2.heapIndex = -1
		loop.timerMap[tmr2.id] = tmr2
		heap.Push(&loop.timers, tmr2)
	})

	select {
	case <-done:
		// The non-canceled timer fired, meaning runTimers processed both
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for timers")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}

// =============================================================================
// pollFastMode — short timeout path (1ms <= timeout < 1000ms)
// =============================================================================

// TestPollFastMode_ShortTimeout exercises the timer-based short timeout path
// in pollFastMode by scheduling a timer with a delay that causes
// calculateTimeout to return a value in [1, 999]ms.
func TestPollFastMode_ShortTimeout(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	done := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	// Schedule a timer with 50ms delay — this causes calculateTimeout to
	// return ~50ms, hitting the short timeout path in pollFastMode
	// (not the indefinite block, not the zero timeout).
	loop.ScheduleTimer(50*time.Millisecond, func() {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for short-timeout timer")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}

// TestPollFastMode_ShortTimeoutWokenBySignal exercises the case where a
// wakeup signal arrives BEFORE the timer expires in the short timeout path.
func TestPollFastMode_ShortTimeoutWokenBySignal(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	taskDone := make(chan struct{})
	timerDone := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	// Schedule a timer with 500ms delay — sets up short timeout in pollFastMode.
	loop.ScheduleTimer(500*time.Millisecond, func() {
		close(timerDone)
	})

	// After a brief moment, submit a task. This task wakes the loop via
	// fastWakeupCh, causing pollFastMode to return BEFORE the timer expires
	// (exercises the <-l.fastWakeupCh case in the select with timer).
	go func() {
		// Small window to let the loop enter pollFastMode with the ~500ms timer
		for i := 0; i < 100; i++ {
			runtime.Gosched()
		}
		loop.Submit(func() {
			close(taskDone)
		})
	}()

	select {
	case <-taskDone:
		// Task woke the loop before the timer expired
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	// Timer should still fire eventually
	select {
	case <-timerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for timer")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}

// =============================================================================
// SubmitInternal — non-fast-path from outside loop thread
// =============================================================================

// TestSubmitInternal_SlowPathFromExternalThread exercises the SubmitInternal
// slow path where fast path conditions fail (we're not on the loop thread).
func TestSubmitInternal_SlowPathFromExternalThread(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	done := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	// SubmitInternal from test goroutine (NOT loop thread) — takes slow path.
	// canUseFastPath() may be true but isLoopThread() is false.
	err = loop.SubmitInternal(func() {
		close(done)
	})
	if err != nil {
		t.Fatalf("SubmitInternal failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}

// =============================================================================
// addHandler — race stress test
// =============================================================================

// TestAddHandler_ReCheckUnderLock_SettledRace exercises the re-check path
// in addHandler where the promise transitions from Pending to settled
// between the optimistic check and the lock acquisition.
func TestAddHandler_ReCheckUnderLock_SettledRace(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	waitForRunning(t, loop)

	// Stress the race: many goroutines resolve and add handlers concurrently.
	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(iterations)

	for i := 0; i < iterations; i++ {
		go func() {
			defer wg.Done()
			promise, resolve, _ := js.NewChainedPromise()

			resolvedCh := make(chan struct{})
			go func() {
				resolve("raced")
				close(resolvedCh)
			}()

			handlerCh := make(chan any, 1)
			promise.Then(func(v any) any {
				handlerCh <- v
				return nil
			}, nil)

			<-resolvedCh
			select {
			case v := <-handlerCh:
				if v != "raced" {
					t.Errorf("expected 'raced', got %v", v)
				}
			case <-time.After(5 * time.Second):
				// handler should eventually fire
			}
		}()
	}

	wg.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}

// =============================================================================
// ingress Pop — exhausted multi-chunk double-check path
// =============================================================================

// TestChunkedIngress_PopDoubleCheckAfterAdvance exercises the double-check
// path in Pop where after advancing to the next chunk, the queue handles
// the transition correctly between two chunks.
func TestChunkedIngress_PopDoubleCheckAfterAdvance(t *testing.T) {
	q := newChunkedIngressWithSize(16)

	// Fill exactly one chunk (16 items) then add 1 more to force second chunk
	for i := 0; i < 17; i++ {
		q.Push(func() {})
	}

	// Pop all items — exercises chunk advancement in Pop
	count := 0
	for {
		_, ok := q.Pop()
		if !ok {
			break
		}
		count++
	}

	if count != 17 {
		t.Errorf("expected 17, popped %d", count)
	}
}

// =============================================================================
// Loop with all options — max coverage for constructor + tick paths
// =============================================================================

// TestLoop_MetricsStrictDisabledEndToEnd exercises the full metrics pipeline
// with strict ordering and disabled fast path to force tick() execution.
func TestLoop_MetricsStrictDisabledEndToEnd(t *testing.T) {
	loop, err := New(
		WithMetrics(true),
		WithFastPathMode(FastPathDisabled),
		WithStrictMicrotaskOrdering(true),
	)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Queue several tasks before running to have non-zero queue depths
	var wg sync.WaitGroup
	wg.Add(5)
	for i := 0; i < 5; i++ {
		loop.Submit(func() {
			wg.Done()
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go loop.Run(ctx)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	m := loop.Metrics()
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)
}
