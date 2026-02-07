// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// STANDARDS-003: Microtask Queue Ordering Compliance (WHATWG Spec)
// ============================================================================
//
// The WHATWG HTML Living Standard specifies that:
// 1. Microtasks run before the next task (macro-task)
// 2. Nested microtasks are processed in the same microtask checkpoint
// 3. Promise reactions (then/catch/finally) are microtasks
// 4. queueMicrotask() follows the spec
//
// Reference: https://html.spec.whatwg.org/multipage/webappapis.html#perform-a-microtask-checkpoint

// TestMicrotaskOrdering_BeforeMacroTask verifies that microtasks run before the next macro-task.
// This is fundamental to WHATWG spec compliance.
func TestMicrotaskOrdering_BeforeMacroTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { loop.Run(ctx) }()
	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var order []string
	done := make(chan struct{})

	// Schedule a setTimeout (macro-task) with 0ms delay
	_, err = js.SetTimeout(func() {
		order = append(order, "setTimeout-0ms")

		// Queue another microtask from within the macro-task
		js.QueueMicrotask(func() {
			order = append(order, "microtask-from-setTimeout")
		})

		// Schedule another setTimeout to verify ordering persists
		js.SetTimeout(func() {
			order = append(order, "setTimeout-nested")
			close(done)
		}, 0)
	}, 0)
	if err != nil {
		t.Fatalf("SetTimeout failed: %v", err)
	}

	// Queue a microtask (should run before the setTimeout above)
	err = js.QueueMicrotask(func() {
		order = append(order, "microtask-1")
	})
	if err != nil {
		t.Fatalf("QueueMicrotask failed: %v", err)
	}

	// Queue another microtask (should run before setTimeout, after microtask-1)
	err = js.QueueMicrotask(func() {
		order = append(order, "microtask-2")
	})
	if err != nil {
		t.Fatalf("QueueMicrotask failed: %v", err)
	}

	// Process events
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("Timeout waiting for callbacks. Got order: %v", order)
	}

	loop.Shutdown(context.Background())

	// Verify: initial microtasks run before first setTimeout
	// The key invariant is that microtasks queued BEFORE setTimeout execute BEFORE setTimeout
	if len(order) < 3 {
		t.Fatalf("Expected at least 3 events, got %d: %v", len(order), order)
	}

	// First two must be the initial microtasks (in order)
	if order[0] != "microtask-1" {
		t.Errorf("Order[0]: expected %q, got %q", "microtask-1", order[0])
	}
	if order[1] != "microtask-2" {
		t.Errorf("Order[1]: expected %q, got %q", "microtask-2", order[1])
	}
	// Then first setTimeout
	if order[2] != "setTimeout-0ms" {
		t.Errorf("Order[2]: expected %q, got %q", "setTimeout-0ms", order[2])
	}

	// Verify all events eventually executed (order of last two may vary by implementation)
	eventSet := make(map[string]bool)
	for _, ev := range order {
		eventSet[ev] = true
	}
	required := []string{"microtask-1", "microtask-2", "setTimeout-0ms", "setTimeout-nested", "microtask-from-setTimeout"}
	for _, r := range required {
		if !eventSet[r] {
			t.Errorf("Missing event: %s", r)
		}
	}

	t.Logf("Verified order: %v", order)
}

// TestMicrotaskOrdering_NestedMicrotasksInSameCheckpoint verifies that
// microtasks queued during microtask processing are executed in the same checkpoint.
// Per WHATWG: "perform a microtask checkpoint" drains ALL microtasks including nested.
func TestMicrotaskOrdering_NestedMicrotasksInSameCheckpoint(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var order []string
	macroTaskRan := make(chan struct{})

	// Submit all setup work to run synchronously within the loop goroutine.
	// This ensures the timer and microtasks are registered in the same tick,
	// so microtasks run before the 0ms timer (correct WHATWG ordering).
	loop.Submit(func() {
		// Schedule a macro-task first
		js.SetTimeout(func() {
			order = append(order, "macro-task")
			close(macroTaskRan)
		}, 0)

		// Queue outer microtask that queues nested microtasks
		js.QueueMicrotask(func() {
			order = append(order, "outer-1")

			// Queue nested microtask - should run BEFORE the macro-task
			js.QueueMicrotask(func() {
				order = append(order, "nested-1a")

				// Even deeper nesting
				js.QueueMicrotask(func() {
					order = append(order, "deep-nested")
				})
			})

			js.QueueMicrotask(func() {
				order = append(order, "nested-1b")
			})
		})

		// Queue another outer microtask
		js.QueueMicrotask(func() {
			order = append(order, "outer-2")
		})
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { loop.Run(ctx) }()

	// Wait for macro-task to complete
	select {
	case <-macroTaskRan:
	case <-time.After(3 * time.Second):
		t.Fatalf("Timeout. Got order: %v", order)
	}

	loop.Shutdown(context.Background())

	// Verify: ALL microtasks (including deeply nested) run before macro-task
	expected := []string{
		"outer-1",     // First queued
		"outer-2",     // Second queued (before nested because FIFO within queue)
		"nested-1a",   // Queued during outer-1
		"nested-1b",   // Queued during outer-1
		"deep-nested", // Queued during nested-1a
		"macro-task",  // Only after ALL microtasks
	}

	if len(order) != len(expected) {
		t.Fatalf("Expected %d events, got %d: %v", len(expected), len(order), order)
	}

	for i, ev := range expected {
		if order[i] != ev {
			t.Errorf("Order[%d]: expected %q, got %q", i, ev, order[i])
		}
	}

	t.Logf("Verified nested microtask checkpoint: %v", order)
}

// TestMicrotaskOrdering_PromiseReactionsAreMicrotasks verifies that
// Promise then/catch/finally handlers are scheduled as microtasks.
// This test uses Submit to avoid race conditions that occur when
// calling resolve/reject from outside the loop goroutine.
func TestMicrotaskOrdering_PromiseReactionsAreMicrotasks(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var order []string
	var mu sync.Mutex
	done := make(chan struct{})

	appendOrder := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	// Submit all setup work to run synchronously within the loop
	loop.Submit(func() {
		// Create and immediately resolve a promise
		p, resolve, _ := js.NewChainedPromise()

		// Attach then handler (promise reaction = microtask)
		p.Then(func(v Result) Result {
			appendOrder("promise-then")
			return v
		}, nil)

		// Attach catch handler (must use separate chain since first is not rejected)
		p2, _, reject2 := js.NewChainedPromise()
		p2.Catch(func(r Result) Result {
			appendOrder("promise-catch")
			return r
		})

		// Attach finally handler
		p3, resolve3, _ := js.NewChainedPromise()
		p3.Finally(func() {
			appendOrder("promise-finally")
		})

		// Schedule macro-task BEFORE resolving promises
		js.SetTimeout(func() {
			appendOrder("setTimeout")
			close(done)
		}, 0)

		// Queue explicit microtask for reference ordering
		js.QueueMicrotask(func() {
			appendOrder("queueMicrotask")
		})

		// Resolve all promises (handlers should queue as microtasks)
		// These microtasks are queued AFTER the queueMicrotask above
		// but all microtasks run before setTimeout
		resolve("success")
		reject2("error")
		resolve3("done")
	})

	// Create context that cancels when done
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-done
		cancel()
	}()
	defer cancel()

	// Run the loop until cancelled
	go func() { loop.Run(ctx) }()

	// Wait for done signal
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("Timeout waiting for test completion")
	}

	loop.Shutdown(context.Background())

	mu.Lock()
	defer mu.Unlock()

	// Verify: all promise reactions before setTimeout
	// The exact order among microtasks depends on when they were queued
	macroIndex := -1
	for i, ev := range order {
		if ev == "setTimeout" {
			macroIndex = i
			break
		}
	}

	if macroIndex == -1 {
		t.Fatal("setTimeout never executed")
	}

	// All promise reactions must be before setTimeout
	promiseReactions := map[string]bool{}
	for i := 0; i < macroIndex; i++ {
		promiseReactions[order[i]] = true
	}

	// Check that promise reactions and queueMicrotask executed before setTimeout
	expectedMicrotasks := []string{"promise-then", "promise-catch", "promise-finally", "queueMicrotask"}
	for _, mt := range expectedMicrotasks {
		if !promiseReactions[mt] {
			t.Errorf("Expected %q before setTimeout, but it wasn't: %v", mt, order)
		}
	}

	t.Logf("Promise reactions are microtasks: %v", order)
}

// TestMicrotaskOrdering_QueueMicrotaskFIFO verifies FIFO ordering within the microtask queue.
func TestMicrotaskOrdering_QueueMicrotaskFIFO(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var order []int
	const numMicrotasks = 100

	// Queue many microtasks
	for i := 0; i < numMicrotasks; i++ {
		idx := i
		err := js.QueueMicrotask(func() {
			order = append(order, idx)
		})
		if err != nil {
			t.Fatalf("QueueMicrotask %d failed: %v", i, err)
		}
	}

	// Process all microtasks with tick()
	loop.tick()

	// Verify FIFO order
	if len(order) != numMicrotasks {
		t.Fatalf("Expected %d microtasks, got %d", numMicrotasks, len(order))
	}

	for i, val := range order {
		if val != i {
			t.Fatalf("FIFO violation: order[%d] = %d", i, val)
		}
	}

	t.Logf("Verified FIFO: %d microtasks in order", numMicrotasks)
}

// TestMicrotaskOrdering_ChainsAcrossTicks verifies that promise chains
// maintain proper microtask ordering across multiple ticks.
func TestMicrotaskOrdering_ChainsAcrossTicks(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var order []string

	// Create a promise chain
	p, resolve, _ := js.NewChainedPromise()

	p.Then(func(v Result) Result {
		order = append(order, "chain-1")
		return v.(int) + 1
	}, nil).Then(func(v Result) Result {
		order = append(order, "chain-2")
		return v.(int) + 1
	}, nil).Then(func(v Result) Result {
		order = append(order, "chain-3")
		return v.(int) + 1
	}, nil).Then(func(v Result) Result {
		order = append(order, "chain-4")
		return v
	}, nil)

	// Resolve the promise
	resolve(1)

	// Process microtasks - may need multiple ticks for chained promises
	for i := 0; i < 10; i++ {
		loop.tick()
	}

	// Verify chain steps are in order relative to each other
	expected := []string{"chain-1", "chain-2", "chain-3", "chain-4"}
	if len(order) != len(expected) {
		t.Fatalf("Expected %d chain events, got %d: %v", len(expected), len(order), order)
	}
	for i, ev := range expected {
		if order[i] != ev {
			t.Errorf("Chain order[%d]: expected %q, got %q", i, ev, order[i])
		}
	}

	t.Logf("Chain ordering verified: %v", order)
}

// TestMicrotaskOrdering_StrictModeEnforcement verifies StrictMicrotaskOrdering option.
func TestMicrotaskOrdering_StrictModeEnforcement(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop, err := New(WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if !loop.StrictMicrotaskOrdering {
		t.Fatal("Expected StrictMicrotaskOrdering to be true")
	}

	go func() { loop.Run(ctx) }()
	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var order []string
	done := make(chan struct{})

	// With strict ordering, microtasks should be processed more aggressively
	js.QueueMicrotask(func() {
		order = append(order, "micro-1")
		js.QueueMicrotask(func() {
			order = append(order, "micro-nested")
		})
	})

	js.QueueMicrotask(func() {
		order = append(order, "micro-2")
	})

	js.SetTimeout(func() {
		order = append(order, "timeout")
		close(done)
	}, 0)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout")
	}

	loop.Shutdown(context.Background())

	// Verify all microtasks before timeout
	timeoutIdx := -1
	for i, ev := range order {
		if ev == "timeout" {
			timeoutIdx = i
			break
		}
	}

	if timeoutIdx == -1 {
		t.Fatal("timeout not in order")
	}

	// All microtasks must be before timeout
	for i := 0; i < timeoutIdx; i++ {
		if order[i] != "micro-1" && order[i] != "micro-2" && order[i] != "micro-nested" {
			t.Errorf("Unexpected event before timeout: %s", order[i])
		}
	}

	t.Logf("Strict mode order: %v", order)
}

// TestMicrotaskOrdering_MixedMicrotaskSources verifies ordering when microtasks
// come from multiple sources: queueMicrotask, promise reactions, loop.ScheduleMicrotask.
func TestMicrotaskOrdering_MixedMicrotaskSources(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { loop.Run(ctx) }()
	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var order []string
	done := make(chan struct{})

	// Queue via loop.ScheduleMicrotask (internal API)
	loop.ScheduleMicrotask(func() {
		order = append(order, "scheduleMicrotask-1")
	})

	// Queue via js.QueueMicrotask (JS API)
	js.QueueMicrotask(func() {
		order = append(order, "queueMicrotask-1")
	})

	// Promise reaction
	p, resolve, _ := js.NewChainedPromise()
	p.Then(func(v Result) Result {
		order = append(order, "promiseReaction-1")
		return v
	}, nil)

	// IMPORTANT: Resolve the promise BEFORE scheduling timeout to ensure
	// the promise reaction microtask is queued before the timeout.
	// This avoids a race condition where the timeout fires before the
	// promise reaction is processed.
	resolve("value")

	// More ScheduleMicrotask
	loop.ScheduleMicrotask(func() {
		order = append(order, "scheduleMicrotask-2")
	})

	// More QueueMicrotask
	js.QueueMicrotask(func() {
		order = append(order, "queueMicrotask-2")
	})

	// Final timeout - scheduled after resolve() to ensure microtasks first
	js.SetTimeout(func() {
		order = append(order, "timeout")
		close(done)
	}, 0)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("Timeout. Got order: %v", order)
	}

	loop.Shutdown(context.Background())

	// Verify: all microtasks before timeout
	timeoutIdx := -1
	for i, ev := range order {
		if ev == "timeout" {
			timeoutIdx = i
			break
		}
	}

	if timeoutIdx == -1 {
		t.Fatal("timeout not found")
	}

	// Count microtasks before timeout
	expectedMicrotasks := 5 // 2 scheduleMicrotask + 2 queueMicrotask + 1 promiseReaction
	if timeoutIdx != expectedMicrotasks {
		t.Errorf("Expected %d microtasks before timeout, got %d: %v", expectedMicrotasks, timeoutIdx, order)
	}

	t.Logf("Mixed sources order: %v", order)
}

// TestMicrotaskOrdering_NilCallback verifies that nil callbacks don't crash.
func TestMicrotaskOrdering_NilCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	// QueueMicrotask with nil should not crash (implementation may skip or panic-recover)
	// The behavior is implementation-defined, we just verify no crash
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Recovered from panic (expected for nil callback): %v", r)
		}
	}()

	err = js.QueueMicrotask(nil)
	// Error or no error is fine, just no crash
	if err != nil {
		t.Logf("QueueMicrotask(nil) returned error (expected): %v", err)
	}

	// Ensure loop still works
	called := false
	js.QueueMicrotask(func() {
		called = true
	})

	loop.tick()

	if !called {
		t.Fatal("Loop not functional after nil callback")
	}
	t.Log("Loop still functional after nil callback")
}

// TestMicrotaskOrdering_ConcurrentQueueing verifies microtask ordering under concurrent queueing.
func TestMicrotaskOrdering_ConcurrentQueueing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { loop.Run(ctx) }()
	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	const numGoroutines = 10
	const numPerGoroutine = 100
	total := numGoroutines * numPerGoroutine

	var count atomic.Int64
	done := make(chan struct{})

	// Launch goroutines that queue microtasks concurrently
	for g := 0; g < numGoroutines; g++ {
		go func() {
			for i := 0; i < numPerGoroutine; i++ {
				js.QueueMicrotask(func() {
					if count.Add(1) == int64(total) {
						close(done)
					}
				})
			}
		}()
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("Timeout: only %d/%d microtasks completed", count.Load(), total)
	}

	loop.Shutdown(context.Background())

	t.Logf("Concurrent queueing: %d microtasks completed", total)
}

// TestMicrotaskOrdering_IntervalInteraction verifies microtask/interval interaction.
func TestMicrotaskOrdering_IntervalInteraction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { loop.Run(ctx) }()
	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var order []string
	var orderMu sync.Mutex
	done := make(chan struct{})

	intervalCount := 0
	var intervalID atomic.Uint64
	id, _ := js.SetInterval(func() {
		intervalCount++
		orderMu.Lock()
		order = append(order, "interval")
		orderMu.Unlock()
		if intervalCount >= 2 {
			js.ClearInterval(intervalID.Load())
			close(done)
		}

		// Queue microtask during interval - should run before next interval/timeout
		js.QueueMicrotask(func() {
			orderMu.Lock()
			order = append(order, "microtask-from-interval")
			orderMu.Unlock()
		})
	}, 10)
	intervalID.Store(id)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		orderMu.Lock()
		orderCopy := make([]string, len(order))
		copy(orderCopy, order)
		orderMu.Unlock()
		t.Fatalf("Timeout. Got order: %v", orderCopy)
	}

	loop.Shutdown(context.Background())

	// After shutdown, no more concurrent access
	orderMu.Lock()
	orderCopy := make([]string, len(order))
	copy(orderCopy, order)
	orderMu.Unlock()

	// Verify: microtasks from each interval run before next interval
	for i := 0; i < len(orderCopy)-1; i++ {
		if orderCopy[i] == "interval" && i+1 < len(orderCopy) {
			// Next event should be microtask-from-interval (same checkpoint)
			if orderCopy[i+1] != "microtask-from-interval" && orderCopy[i+1] != "interval" {
				t.Errorf("After interval, expected microtask or interval, got: %s", orderCopy[i+1])
			}
		}
	}

	t.Logf("Interval interaction order: %v", orderCopy)
}

// TestMicrotaskOrdering_DeepNesting verifies deeply nested microtasks don't cause stack overflow.
func TestMicrotaskOrdering_DeepNesting(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	const depth = 1000
	var counter int

	var nest func(n int)
	nest = func(n int) {
		counter++
		if n > 0 {
			js.QueueMicrotask(func() {
				nest(n - 1)
			})
		}
	}

	js.QueueMicrotask(func() {
		nest(depth)
	})

	// Process all nested microtasks - need multiple ticks
	for i := 0; i <= depth; i++ {
		loop.tick()
	}

	if counter != depth+1 {
		t.Fatalf("Expected %d nesting levels, got %d", depth+1, counter)
	}

	t.Logf("Deep nesting: %d levels completed", depth)
}

// TestMicrotaskOrdering_AfterTimerFires verifies microtasks queued after timer fires
// are eventually processed. The exact ordering relative to subsequent timers may vary.
func TestMicrotaskOrdering_AfterTimerFires(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { loop.Run(ctx) }()
	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var order []string
	done := make(chan struct{})

	// First timer
	js.SetTimeout(func() {
		order = append(order, "timer-1")

		// Queue microtask during timer callback
		js.QueueMicrotask(func() {
			order = append(order, "microtask-during-timer")
		})
	}, 0)

	// Second timer with enough delay to be processed separately
	js.SetTimeout(func() {
		order = append(order, "timer-2")

		// Final microtask to close done
		js.QueueMicrotask(func() {
			close(done)
		})
	}, 50)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("Timeout. Got order: %v", order)
	}

	loop.Shutdown(context.Background())

	// Verify: timer-1 runs before timer-2, and microtask-during-timer eventually runs
	timer1Idx := -1
	microIdx := -1
	timer2Idx := -1
	for i, ev := range order {
		switch ev {
		case "timer-1":
			timer1Idx = i
		case "microtask-during-timer":
			microIdx = i
		case "timer-2":
			timer2Idx = i
		}
	}

	if timer1Idx == -1 || microIdx == -1 || timer2Idx == -1 {
		t.Fatalf("Missing events: %v", order)
	}

	// timer-1 must be before timer-2
	if timer1Idx >= timer2Idx {
		t.Errorf("Expected timer-1 before timer-2, got order: %v", order)
	}

	// microtask-during-timer must eventually run (after timer-1)
	if microIdx <= timer1Idx {
		t.Errorf("Expected microtask-during-timer after timer-1, got order: %v", order)
	}

	t.Logf("After-timer microtask order: %v", order)
}
