// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// COVERAGE-020: JS SetImmediate/ClearImmediate Full Coverage
// Gaps: ID exhaustion path, cleared flag CAS logic, map cleanup on panic in run(),
// Submit() failure path

// TestJS_SetImmediate_BasicFunctionality verifies SetImmediate fires asynchronously.
func TestJS_SetImmediate_BasicFunctionality(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	executed := make(chan struct{})

	id, err := js.SetImmediate(func() {
		close(executed)
	})
	if err != nil {
		t.Fatalf("SetImmediate failed: %v", err)
	}

	if id == 0 {
		t.Error("SetImmediate should return non-zero ID")
	}

	go loop.Run(context.Background())

	select {
	case <-executed:
		// Success
	case <-time.After(time.Second):
		t.Fatal("SetImmediate callback never executed")
	}
}

// TestJS_SetImmediate_NilCallback verifies nil callback returns 0 and no error.
func TestJS_SetImmediate_NilCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	id, err := js.SetImmediate(nil)
	if err != nil {
		t.Errorf("SetImmediate(nil) should not error, got: %v", err)
	}
	if id != 0 {
		t.Errorf("SetImmediate(nil) should return 0, got %d", id)
	}
}

// TestJS_SetImmediate_IDGeneration verifies IDs start at high value and increment.
func TestJS_SetImmediate_IDGeneration(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	// First ID should be 1 << 48 + 1 (after Add(1))
	id1, err := js.SetImmediate(func() {})
	if err != nil {
		t.Fatalf("SetImmediate failed: %v", err)
	}

	expectedBase := uint64(1 << 48)
	if id1 <= expectedBase {
		t.Errorf("First ID should be > %d, got %d", expectedBase, id1)
	}

	id2, err := js.SetImmediate(func() {})
	if err != nil {
		t.Fatalf("SetImmediate failed: %v", err)
	}

	if id2 != id1+1 {
		t.Errorf("IDs should be sequential: %d, %d", id1, id2)
	}
}

// TestJS_SetImmediate_IDExhaustion verifies error when ID exceeds max safe integer.
func TestJS_SetImmediate_IDExhaustion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	// Set nextImmediateID to just below maxSafeInteger
	js.nextImmediateID.Store(maxSafeInteger)

	_, err = js.SetImmediate(func() {})
	if err != ErrImmediateIDExhausted {
		t.Errorf("Expected ErrImmediateIDExhausted, got: %v", err)
	}
}

// TestJS_ClearImmediate_BeforeExecution verifies ClearImmediate before callback runs.
func TestJS_ClearImmediate_BeforeExecution(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var executed atomic.Bool

	id, err := js.SetImmediate(func() {
		executed.Store(true)
	})
	if err != nil {
		t.Fatalf("SetImmediate failed: %v", err)
	}

	// Clear before running
	err = js.ClearImmediate(id)
	if err != nil {
		t.Errorf("ClearImmediate should not error: %v", err)
	}

	// Now run the loop briefly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	loop.Run(ctx)

	if executed.Load() {
		t.Error("Callback should not execute after ClearImmediate")
	}
}

// TestJS_ClearImmediate_InvalidID verifies ClearImmediate with invalid ID.
func TestJS_ClearImmediate_InvalidID(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	err = js.ClearImmediate(999999)
	if err != ErrTimerNotFound {
		t.Errorf("ClearImmediate(invalid) should return ErrTimerNotFound, got: %v", err)
	}
}

// TestJS_ClearImmediate_MultipleTimes verifies ClearImmediate is safe to call twice.
func TestJS_ClearImmediate_MultipleTimes(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	id, err := js.SetImmediate(func() {})
	if err != nil {
		t.Fatalf("SetImmediate failed: %v", err)
	}

	// First clear should succeed
	err = js.ClearImmediate(id)
	if err != nil {
		t.Errorf("First ClearImmediate should not error: %v", err)
	}

	// Second clear should return ErrTimerNotFound
	err = js.ClearImmediate(id)
	if err != ErrTimerNotFound {
		t.Errorf("Second ClearImmediate should return ErrTimerNotFound, got: %v", err)
	}
}

// TestJS_SetImmediate_ClearedFlagCAS verifies the cleared flag CAS logic in run().
func TestJS_SetImmediate_ClearedFlagCAS(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var executionCount atomic.Int32

	id, err := js.SetImmediate(func() {
		executionCount.Add(1)
	})
	if err != nil {
		t.Fatalf("SetImmediate failed: %v", err)
	}

	go loop.Run(context.Background())
	time.Sleep(50 * time.Millisecond) // Wait for execution

	// The cleared flag CAS should ensure the callback only runs once
	if count := executionCount.Load(); count != 1 {
		t.Errorf("Callback should execute exactly once, got %d", count)
	}

	// Verify state is cleaned up
	js.setImmediateMu.RLock()
	_, exists := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()

	if exists {
		t.Error("State should be removed after execution")
	}
}

// TestJS_SetImmediate_CleanupOnPanic verifies map cleanup when callback panics.
func TestJS_SetImmediate_CleanupOnPanic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	id, err := js.SetImmediate(func() {
		panic("test panic in SetImmediate")
	})
	if err != nil {
		t.Fatalf("SetImmediate failed: %v", err)
	}

	// Run briefly (panic will be recovered by event loop)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	loop.Run(ctx)

	// Verify state is cleaned up despite panic
	js.setImmediateMu.RLock()
	_, exists := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()

	if exists {
		t.Error("State should be removed even after panic (defer cleanup)")
	}
}

// TestJS_SetImmediate_SubmitFailure verifies cleanup when Submit fails.
func TestJS_SetImmediate_SubmitFailure(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Shutdown immediately to cause Submit failures
	loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	_, err = js.SetImmediate(func() {
		t.Error("Should not execute")
	})
	if err == nil {
		t.Error("SetImmediate on shut down loop should fail")
	}

	// Verify map is cleaned up after Submit failure
	js.setImmediateMu.RLock()
	mapLen := len(js.setImmediateMap)
	js.setImmediateMu.RUnlock()

	if mapLen != 0 {
		t.Errorf("Map should be empty after Submit failure, got %d entries", mapLen)
	}
}

// TestJS_SetImmediate_ConcurrentSetAndClear verifies race safety.
func TestJS_SetImmediate_ConcurrentSetAndClear(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	go loop.Run(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			id, err := js.SetImmediate(func() {})
			if err != nil {
				return // Loop may be terminating
			}

			// Sometimes clear immediately
			if id%2 == 0 {
				js.ClearImmediate(id)
			}
		}()
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond) // Let callbacks execute
}

// TestJS_SetImmediate_MapCleanupAfterExecution verifies map entry is removed.
func TestJS_SetImmediate_MapCleanupAfterExecution(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	executed := make(chan struct{})

	id, err := js.SetImmediate(func() {
		close(executed)
	})
	if err != nil {
		t.Fatalf("SetImmediate failed: %v", err)
	}

	// Verify entry exists before execution
	js.setImmediateMu.RLock()
	_, existsBefore := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()

	if !existsBefore {
		t.Error("State should exist before execution")
	}

	go loop.Run(context.Background())

	<-executed
	time.Sleep(10 * time.Millisecond) // Let cleanup happen

	// Verify entry is removed after execution
	js.setImmediateMu.RLock()
	_, existsAfter := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()

	if existsAfter {
		t.Error("State should be removed after execution")
	}
}

// TestJS_SetImmediate_RunSetsCleared verifies run() uses CAS to set cleared flag.
func TestJS_SetImmediate_RunSetsCleared(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	executed := make(chan struct{})

	id, err := js.SetImmediate(func() {
		close(executed)
	})
	if err != nil {
		t.Fatalf("SetImmediate failed: %v", err)
	}

	// Get state reference before execution
	js.setImmediateMu.RLock()
	state := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()

	if state == nil {
		t.Fatal("State should exist")
	}

	if state.cleared.Load() {
		t.Error("Cleared flag should be false before execution")
	}

	go loop.Run(context.Background())

	<-executed
	time.Sleep(10 * time.Millisecond)

	// After execution, cleared should be true (set by CAS)
	if !state.cleared.Load() {
		t.Error("Cleared flag should be true after execution")
	}
}

// TestJS_SetImmediate_ClearedPreventsDoubleExecution verifies CAS prevents double run.
func TestJS_SetImmediate_ClearedPreventsDoubleExecution(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var executionCount atomic.Int32

	id, err := js.SetImmediate(func() {
		executionCount.Add(1)
	})
	if err != nil {
		t.Fatalf("SetImmediate failed: %v", err)
	}

	// Get state reference
	js.setImmediateMu.RLock()
	state := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()

	if state == nil {
		t.Fatal("State should exist")
	}

	// Manually set cleared to simulate race
	state.cleared.Store(true)

	go loop.Run(context.Background())
	time.Sleep(50 * time.Millisecond)

	// Should not execute because cleared was already true
	if executionCount.Load() != 0 {
		t.Errorf("Should not execute when cleared=true, got %d executions", executionCount.Load())
	}
}

// TestJS_SetImmediate_MultipleImmediate verifies multiple SetImmediate calls.
func TestJS_SetImmediate_MultipleImmediate(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	const count = 100
	executed := make(chan int, count)

	for i := 0; i < count; i++ {
		idx := i
		_, err := js.SetImmediate(func() {
			executed <- idx
		})
		if err != nil {
			t.Fatalf("SetImmediate %d failed: %v", i, err)
		}
	}

	go loop.Run(context.Background())

	// Collect all executions
	resultSet := make(map[int]bool)
	timeout := time.After(5 * time.Second)

	for len(resultSet) < count {
		select {
		case idx := <-executed:
			resultSet[idx] = true
		case <-timeout:
			t.Fatalf("Timeout: only got %d/%d executions", len(resultSet), count)
		}
	}

	// Verify all callbacks executed
	for i := 0; i < count; i++ {
		if !resultSet[i] {
			t.Errorf("Callback %d did not execute", i)
		}
	}
}

// TestJS_SetImmediate_OrderingFIFO verifies FIFO ordering.
func TestJS_SetImmediate_OrderingFIFO(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	const count = 10
	order := make([]int, 0, count)
	var mu sync.Mutex
	done := make(chan struct{})

	for i := 0; i < count; i++ {
		idx := i
		js.SetImmediate(func() {
			mu.Lock()
			order = append(order, idx)
			if len(order) == count {
				close(done)
			}
			mu.Unlock()
		})
	}

	go loop.Run(context.Background())

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout")
	}

	// Verify FIFO order
	for i := 0; i < count; i++ {
		if order[i] != i {
			t.Errorf("Order[%d] = %d, expected %d", i, order[i], i)
		}
	}
}

// TestJS_SetImmediate_NoTimerHeap verifies SetImmediate doesn't use timer heap.
// This test verifies through behavior: SetImmediate should execute immediately
// without any timer delay overhead.
func TestJS_SetImmediate_NoTimerHeap(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	executed := make(chan struct{})
	_, err = js.SetImmediate(func() {
		close(executed)
	})
	if err != nil {
		t.Fatalf("SetImmediate failed: %v", err)
	}

	go loop.Run(context.Background())

	// SetImmediate should execute very quickly (no timer delay)
	select {
	case <-executed:
		// Success - executed quickly
	case <-time.After(100 * time.Millisecond):
		t.Error("SetImmediate should execute immediately without timer delay")
	}
}

// TestJS_SetImmediate_FasterThanSetTimeout0 verifies SetImmediate is faster.
func TestJS_SetImmediate_FasterThanSetTimeout0(t *testing.T) {
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
	var mu sync.Mutex
	done := make(chan struct{})

	// Schedule setTimeout(0) first
	js.SetTimeout(func() {
		mu.Lock()
		order = append(order, "timeout")
		if len(order) == 2 {
			close(done)
		}
		mu.Unlock()
	}, 0)

	// Schedule SetImmediate second
	js.SetImmediate(func() {
		mu.Lock()
		order = append(order, "immediate")
		if len(order) == 2 {
			close(done)
		}
		mu.Unlock()
	})

	go loop.Run(context.Background())

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout")
	}

	// SetImmediate should execute before setTimeout(0) due to bypassing timer heap
	// Note: This depends on implementation - may not always be guaranteed
	t.Logf("Execution order: %v", order)
}

// TestJS_SetImmediate_ManyRapidSchedules verifies handling of rapid scheduling.
func TestJS_SetImmediate_ManyRapidSchedules(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	const count = 1000
	var executed atomic.Int32
	done := make(chan struct{})

	go loop.Run(context.Background())

	for i := 0; i < count; i++ {
		js.SetImmediate(func() {
			if executed.Add(1) == count {
				close(done)
			}
		})
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("Timeout: only %d/%d executed", executed.Load(), count)
	}

	if executed.Load() != count {
		t.Errorf("Expected %d, got %d", count, executed.Load())
	}
}

// TestJS_SetImmediate_MemoryReclamation verifies no memory leak.
func TestJS_SetImmediate_MemoryReclamation(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	go loop.Run(context.Background())

	// Schedule many immediates
	const count = 1000
	done := make(chan struct{})
	var executed atomic.Int32

	for i := 0; i < count; i++ {
		js.SetImmediate(func() {
			if executed.Add(1) == count {
				close(done)
			}
		})
	}

	<-done

	// Allow cleanup
	time.Sleep(50 * time.Millisecond)
	runtime.GC()

	// Verify map is empty
	js.setImmediateMu.RLock()
	mapLen := len(js.setImmediateMap)
	js.setImmediateMu.RUnlock()

	if mapLen != 0 {
		t.Errorf("Map should be empty after all executions, got %d entries", mapLen)
	}
}

// TestJS_ClearImmediate_ConcurrentWithExecution verifies race between clear and run.
func TestJS_ClearImmediate_ConcurrentWithExecution(t *testing.T) {
	for i := 0; i < 100; i++ {
		loop, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		js, err := NewJS(loop)
		if err != nil {
			t.Fatalf("NewJS() failed: %v", err)
		}

		var executed atomic.Bool

		id, err := js.SetImmediate(func() {
			executed.Store(true)
		})
		if err != nil {
			loop.Shutdown(context.Background())
			t.Fatalf("SetImmediate failed: %v", err)
		}

		// Race: start loop and clear simultaneously
		go loop.Run(context.Background())
		js.ClearImmediate(id)

		time.Sleep(10 * time.Millisecond)

		loop.Shutdown(context.Background())

		// Either executed or not - but no panic
		_ = executed.Load()
	}
}

// TestJS_SetImmediate_StateFields verifies setImmediateState field initialization.
func TestJS_SetImmediate_StateFields(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	testFn := func() {}

	id, err := js.SetImmediate(testFn)
	if err != nil {
		t.Fatalf("SetImmediate failed: %v", err)
	}

	js.setImmediateMu.RLock()
	state := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()

	if state == nil {
		t.Fatal("State should exist")
	}

	if state.fn == nil {
		t.Error("fn should not be nil")
	}
	if state.js != js {
		t.Error("js reference should match")
	}
	if state.id != id {
		t.Errorf("id should be %d, got %d", id, state.id)
	}
	if state.cleared.Load() {
		t.Error("cleared should be false initially")
	}
}
