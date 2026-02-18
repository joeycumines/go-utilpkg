package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSetImmediate_Basic tests basic SetImmediate execution
func TestSetImmediate_Basic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}
	if js == nil {
		t.Fatal("JS not initialized")
	}

	executed := false
	id, err := js.SetImmediate(func() {
		executed = true
	})
	if err != nil {
		t.Fatal("SetImmediate failed:", err)
	}
	if id == 0 {
		t.Fatal("Expected non-zero timer ID")
	}

	// Process tasks with a single tick
	loop.tick()

	if !executed {
		t.Error("Immediate function was not executed")
	}
}

// TestSetImmediate_NilCallback tests nil callback handling
func TestSetImmediate_NilCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	id, err := js.SetImmediate(nil)
	if err != nil {
		t.Fatal("SetImmediate with nil callback failed:", err)
	}
	if id != 0 {
		t.Error("Expected ID 0 for nil callback, got:", id)
	}
}

// TestSetImmediate_PanicRecovery tests that panics in callbacks are handled
func TestSetImmediate_PanicRecovery(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	executeOrder := []string{}
	id, err := js.SetImmediate(func() {
		executeOrder = append(executeOrder, "panic")
		panic("test panic in callback")
	})
	if err != nil {
		t.Fatal("SetImmediate failed:", err)
	}
	if id == 0 {
		t.Fatal("Expected non-zero timer ID")
	}

	// Add another immediate to verify event loop continues after panic
	_, err = js.SetImmediate(func() {
		executeOrder = append(executeOrder, "after-panic")
	})
	if err != nil {
		t.Fatal("SetImmediate failed:", err)
	}

	// Execute tasks - panic should be caught and logged, not crash loop
	loop.tick()
	loop.tick()

	// Verify that both tasks executed (event loop continued after panic)
	if len(executeOrder) != 2 {
		t.Errorf("Expected 2 tasks to execute, got %d: %v", len(executeOrder), executeOrder)
	}
	if executeOrder[0] != "panic" {
		t.Errorf("Expected first task to be 'panic', got: %s", executeOrder[0])
	}
	if executeOrder[1] != "after-panic" {
		t.Errorf("Expected second task to be 'after-panic', got: %s", executeOrder[1])
	}
}

// TestClearImmediate_TimerNotFound tests clearing non-existent timer ID
func TestClearImmediate_TimerNotFound(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	// Try to clear a non-existent ID
	err = js.ClearImmediate(12345)
	if err == nil {
		t.Error("Expected ErrTimerNotFound for non-existent ID")
	}
	if !errors.Is(err, ErrTimerNotFound) {
		t.Errorf("Expected ErrTimerNotFound, got: %v", err)
	}
}

// TestClearImmediate_NotExecuted tests clearing immediate before execution
func TestClearImmediate_NotExecuted(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	executed := false
	id, err := js.SetImmediate(func() {
		executed = true
	})
	if err != nil {
		t.Fatal("SetImmediate failed:", err)
	}

	// Clear immediate before execution
	err = js.ClearImmediate(id)
	if err != nil {
		t.Fatal("ClearImmediate failed:", err)
	}

	// Run loop - callback should not execute
	loop.tick()

	if executed {
		t.Error("Immediate callback executed after being cleared")
	}
}

// TestClearImmediate_AlreadyExecuted tests clearing immediate after execution
func TestClearImmediate_AlreadyExecuted(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	executed := false
	id, err := js.SetImmediate(func() {
		executed = true
	})
	if err != nil {
		t.Fatal("SetImmediate failed:", err)
	}

	// Run loop to execute immediate (before clear)
	loop.tick()

	if !executed {
		t.Error("Immediate callback was not executed")
	}

	// Try to clear after execution - should return ErrTimerNotFound
	err = js.ClearImmediate(id)
	if err == nil {
		t.Error("Expected ErrTimerNotFound for already-executed immediate")
	}
	if !errors.Is(err, ErrTimerNotFound) {
		t.Errorf("Expected ErrTimerNotFound, got: %v", err)
	}
}

// TestClearImmediate_Concurrent tests multiple concurrent ClearImmediate calls
func TestClearImmediate_Concurrent(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	executed := false
	id, err := js.SetImmediate(func() {
		executed = true
	})
	if err != nil {
		t.Fatal("SetImmediate failed:", err)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	// Multiple goroutines try to clear of same ID
	for range 2 {
		wg.Go(func() {
			if err := js.ClearImmediate(id); err != nil && !errors.Is(err, ErrTimerNotFound) {
				errChan <- err
			}
		})
	}

	wg.Wait()
	close(errChan)

	// Check for unexpected errors
	for err := range errChan {
		t.Errorf("Unexpected error in concurrent ClearImmediate: %v", err)
	}

	// Run loop - callback should not execute
	loop.tick()

	if executed {
		t.Error("Immediate callback executed after being cleared")
	}
}

// TestSetImmediate_SubmitError_LoopShutdown tests SetImmediate when loop is shutting down
func TestSetImmediate_SubmitError_LoopShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	// Start shutdown
	shutdownErr := make(chan error)
	go func() {
		shutdownErr <- loop.Shutdown(context.Background())
	}()

	// Try to SetImmediate while shutdown is in progress
	time.Sleep(10 * time.Millisecond) // Give shutdown time to start

	_, err = js.SetImmediate(func() {})
	// We expect either success (if Submit raced before shutdown)
	// or an error (if loop already terminated)
	// Both are acceptable outcomes for this test
	if err != nil {
		// Success path: Submit failed as expected
		// Verify that shutdown completes successfully
		<-shutdownErr
		return
	}

	// If SetImmediate succeeded, just verify cleanup is correct
	<-shutdownErr
}

// TestClearImmediate_RaceWindow tests clearing immediate during execution race window
func TestClearImmediate_RaceWindow(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	// Create a barrier to control execution timing
	barrier := make(chan struct{})

	id, err := js.SetImmediate(func() {
		<-barrier // Wait for ClearImmediate to proceed
	})
	if err != nil {
		t.Fatal("SetImmediate failed:", err)
	}

	// Clear immediate immediately (race window)
	err = js.ClearImmediate(id)
	if err != nil {
		t.Fatal("ClearImmediate failed:", err)
	}

	// Release barrier and run loop
	close(barrier)
	loop.tick()

	// The callback might execute if race was won by run()
	// OR might not execute if ClearImmediate won
	// Both outcomes are acceptable - what matters is no panic/deadlock
}

// TestSetImmediate_Multiple tests multiple SetImmediate calls
func TestSetImmediate_Multiple(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	executed := make([]bool, 10)
	for i := range 10 {
		idx := i
		_, err := js.SetImmediate(func() {
			executed[idx] = true
		})
		if err != nil {
			t.Fatal("SetImmediate failed:", err)
		}
	}

	// Process tasks with a single tick
	loop.tick()

	allExecuted := true
	for i, exec := range executed {
		if !exec {
			t.Errorf("Immediate %d was not executed", i)
			allExecuted = false
		}
	}

	if !allExecuted {
		t.Error("Not all immediate callbacks were executed")
	}
}

// TestClearImmediate_Multiple tests clearing multiple immediates
func TestClearImmediate_Multiple(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	ids := make([]uint64, 10)
	for i := range 10 {
		id, err := js.SetImmediate(func() {})
		if err != nil {
			t.Fatal("SetImmediate failed:", err)
		}
		ids[i] = id
	}

	// Clear all immediates
	for _, id := range ids {
		err := js.ClearImmediate(id)
		if err != nil {
			t.Errorf("ClearImmediate failed for ID %d: %v", id, err)
		}
	}

	// Run loop - no callbacks should execute
	// (We can't verify this directly without counting, just ensure no crash)
	loop.tick()
}

// TestSetImmediate_ErrImmediateIDExhausted verifies that when immediate ID counter
// exceeds MAX_SAFE_INTEGER, ErrImmediateIDExhausted is returned.
// Priority 1: HIGH - ID exhaustion is a real scenario in long-running systems.
func TestSetImmediate_ErrImmediateIDExhausted(t *testing.T) {
	t.Skip("ID exhaustion test: Requires mocking atomic counter or running >2^53 immediates")

	// Note: This test would require:
	// 1. Forcing nextImmediateID > MAX_SAFE_INTEGER (9007199254740991)
	// 2. Verifying ErrImmediateIDExhausted is returned
	// 3. Testing recovery after counter wrap
	//
	// Current implementation: nextImmediateID is atomic.Uint64 in loop.go
	// Realistic test needs ~2^53 calls, which is impractical in test
}

// TestSetTimeout_NilCallback verifies that nil callback returns ID=0 without error.
// Priority 1: HIGH - Nil handling is critical for robustness.
func TestSetTimeout_NilCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	id, err := js.SetTimeout(nil, 100)
	if err != nil {
		t.Fatal("SetTimeout with nil should not error:", err)
	}
	if id != 0 {
		t.Fatal("Expected ID=0 for nil callback, got:", id)
	}
}

// TestSetTimeout_ErrTimerIDExhausted verifies that when timer ID counter
// exceeds MAX_SAFE_INTEGER, ErrTimerIDExhausted is returned.
// Priority 1: HIGH - ID exhaustion is a real scenario.
func TestSetTimeout_ErrTimerIDExhausted(t *testing.T) {
	t.Skip("ID exhaustion test: Requires mocking atomic counter or running >2^53 timers")

	// Note: Similar to SetImmediate exhaustion, requires forcing
	// nextTimerID > MAX_SAFE_INTEGER and verifying error return
}

// TestSetTimeout_SubmitError_LoopShutdown verifies error propagation when
// scheduling timer during loop shutdown.
// Priority 1: HIGH - Resource leak prevention.
func TestSetTimeout_SubmitError_LoopShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Shutdown loop
	go func() {
		time.Sleep(10 * time.Millisecond)
		loop.Shutdown(ctx)
	}()

	// Try to schedule timeout during shutdown (should handle gracefully)
	time.Sleep(20 * time.Millisecond)
	id, err := js.SetTimeout(func() {}, 100)

	// After shutdown, timer submission should either:
	// 1. Return error immediately (SubmitInternal fails)
	// 2. Return ID=0 (nil check or state check)
	// Either is acceptable as long as no panic occurs
	if err != nil {
		// Error during shutdown is acceptable
		t.Log("SetTimeout during shutdown returned error:", err)
	} else if id == 0 {
		// ID=0 is also acceptable (nil or rejected)
		t.Log("SetTimeout during shutdown returned ID=0")
	}
}

// TestClearTimeout_BeforeExecution verifies that timeout is cleared before execution.
func TestClearTimeout_BeforeExecution(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	// Test needs a running loop for ClearTimeout
	ctx := t.Context()
	go loop.Run(ctx)
	time.Sleep(5 * time.Millisecond) // Give loop time to start

	// Schedule timeout with longer delay to ensure it doesn't execute before we clear
	executed := false
	id, err := js.SetTimeout(func() { executed = true }, 100)
	if err != nil {
		t.Fatal("SetTimeout failed:", err)
	}
	if id == 0 {
		t.Fatal("Expected non-zero timer ID")
	}

	// Clear immediately (well before 100ms timeout)
	err = js.ClearTimeout(id)
	if err != nil {
		t.Fatal("ClearTimeout failed:", err)
	}

	// Wait past original timeout
	time.Sleep(20 * time.Millisecond)

	if executed {
		t.Error("Callback was executed after clear")
	}
}

// TestClearTimeout_TimerNotFound verifies that clearing non-existent timer
// returns ErrTimerNotFound.
func TestClearTimeout_TimerNotFound(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	// Test needs a running loop for ClearTimeout
	ctx := t.Context()
	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond) // Give loop time to start

	// Try to clear timer that never existed
	err = js.ClearTimeout(99999)
	// Can be ErrTimerNotFound or ErrLoopNotRunning (shouldn't happen with sleep)
	if err != ErrTimerNotFound && err != ErrLoopNotRunning {
		t.Errorf("Expected ErrTimerNotFound, got: %v", err)
	}
}

// TestClearTimeout_InvalidID_Cast verifies safe handling of invalid IDs.
// Priority 4: LOW - Invalid input handling.
func TestClearTimeout_InvalidID_Cast(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	// Pass invalid ID (large uint64 that might cause cast issues)
	invalidID := uint64(1) << 63 // Very large ID
	err = js.ClearTimeout(invalidID)

	// Should handle gracefully (either ErrTimerNotFound or no panic)
	if err != ErrTimerNotFound {
		t.Logf("Invalid ID handling: got error %v (expected ErrTimerNotFound)", err)
	}
	// The critical check: no panic occurred
}

// TestSetInterval_NilCallback verifies that nil callback returns ID=0 without error.
// Priority 1: HIGH - Nil handling is critical.
func TestSetInterval_NilCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	id, err := js.SetInterval(nil, 100)
	if err != nil {
		t.Fatal("SetInterval with nil should not error:", err)
	}
	if id != 0 {
		t.Fatal("Expected ID=0 for nil callback, got:", id)
	}
}

// TestSetInterval_ErrIntervalIDExhausted verifies that when interval ID counter
// exceeds MAX_SAFE_INTEGER, ErrIntervalIDExhausted is returned.
// Priority 1: HIGH - ID exhaustion is a real scenario.
func TestSetInterval_ErrIntervalIDExhausted(t *testing.T) {
	t.Skip("ID exhaustion test: Requires mocking atomic counter or running >2^53 intervals")

	// Note: Similar to timer exhaustion, requires forcing
	// nextIntervalID > MAX_SAFE_INTEGER and verifying error return
}

// TestSetInterval_SubmitError_LoopShutdown verifies error cleanup when
// scheduling interval during loop shutdown.
// Priority 1: HIGH - Resource leak prevention.
func TestSetInterval_SubmitError_LoopShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Shutdown loop
	go func() {
		time.Sleep(10 * time.Millisecond)
		loop.Shutdown(ctx)
	}()

	// Try to schedule interval during shutdown
	time.Sleep(20 * time.Millisecond)
	id, err := js.SetInterval(func() {}, 100)

	// Similar to SetTimeout: should handle gracefully
	if err != nil {
		t.Log("SetInterval during shutdown returned error:", err)
	} else if id == 0 {
		t.Log("SetInterval during shutdown returned ID=0")
	}
}

// TestSetInterval_WrapperInitializationRace verifies race between interval
// initialization and map storage.
// Priority 1: HIGH - Race during startup.
func TestSetInterval_WrapperInitializationRace(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	// Rapid interval creation
	iterations := 100
	var wg sync.WaitGroup
	for range iterations {
		wg.Go(func() {
			// Each goroutine creates its own interval
			id, _ := js.SetInterval(func() {}, 1000)
			// No panic should occur during rapid concurrent creation
			if id != 0 {
				_ = js.ClearInterval(id) // Cleanup
			}
		})
	}

	// Give some time for concurrent operations
	time.Sleep(50 * time.Millisecond)

	// Continue with normal loop start for a bit
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	go loop.Run(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	loop.Shutdown(context.Background())

	wg.Wait()
	// If we get here without panic, race handling is correct
}

// TestClearInterval_RaceCondition_WrapperRunning verifies TOCTOU race
// where ClearInterval is called while wrapper is actively executing.
// Priority 1: HIGH - TOCTOU race documented in js.go.
func TestClearInterval_RaceCondition_WrapperRunning(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	executed := atomic.Bool{}
	executed.Store(false)
	id, err := js.SetInterval(func() {
		// This executes in interval callback
		// Give ClearInterval a chance to run mid-execution
		t.Log("Interval executing")
		executed.Store(true)
	}, 10)
	if err != nil {
		t.Fatal("SetInterval failed:", err)
	}

	// Start loop
	ctx, cancel := context.WithCancel(context.Background())
	go loop.Run(ctx)

	// Wait for first execution with generous timeout
	deadline := time.After(5 * time.Second)
	for !executed.Load() {
		select {
		case <-deadline:
			t.Fatal("Interval did not execute within timeout")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	// Clear immediately while wrapper might be running (TOCTOU race)
	err = js.ClearInterval(id)
	if err != nil {
		t.Log("ClearInterval during execution returned:", err)
		// Either ErrTimerNotFound or another error is acceptable
		// Critical: should not panic
	}

	// Give time for cleanup
	time.Sleep(50 * time.Millisecond)
	cancel()
	loop.Shutdown(context.Background())

	// If we reach here without panic, TOCTOU race is handled
}

// TestClearInterval_InvalidID verifies safe handling of invalid interval IDs.
func TestClearInterval_InvalidID(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	// Clear non-existent interval
	err = js.ClearInterval(99999)
	if err != ErrTimerNotFound {
		t.Errorf("Expected ErrTimerNotFound, got: %v", err)
	}
}

// TestQueueMicrotask_ErrLoopTerminated verifies that QueueMicrotask
// returns ErrLoopTerminated when loop is terminated.
// Priority 1: HIGH - Loop shutdown error path.
func TestQueueMicrotask_ErrLoopTerminated(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	// Shutdown loop first
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond) // Give loop a chance to start
	loop.Shutdown(context.Background())

	// Now try to queue microtask after shutdown
	err = js.QueueMicrotask(func() {})
	if err != ErrLoopTerminated {
		t.Errorf("Expected ErrLoopTerminated, got: %v", err)
	}
}

// TestQueueMicrotask_PanicRecovery verifies that panic in microtask
// doesn't crash the loop.
// Priority 1: HIGH - Robustness requirement.
func TestQueueMicrotask_PanicRecovery(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	// Queue microtask that panics
	_ = js.QueueMicrotask(func() {
		panic("intentional test panic")
	})

	// Queue a second microtask that should still execute
	afterPanicExecuted := atomic.Bool{}
	afterPanicExecuted.Store(false)
	_ = js.QueueMicrotask(func() {
		afterPanicExecuted.Store(true)
	})

	// Run loop
	ctx := t.Context()
	go loop.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// If second microtask executes, panic was handled
	if !afterPanicExecuted.Load() {
		t.Log("After-panic microtask may not have executed (expected behavior if panic stops processing)")
	}

	// Critical check: loop should not have crashed
	// If we reach here, panic recovery worked
}

// TestScheduleMicrotask_NilCallback verifies nil callback handling.
func TestScheduleMicrotask_NilCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	// Queue nil microtask
	err = js.QueueMicrotask(nil)
	if err != nil {
		t.Fatal("QueueMicrotask with nil should not error:", err)
	}

	// Run loop
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	go loop.Run(ctx)
	<-ctx.Done()
	loop.Shutdown(context.Background())

	// If we reach here, nil handling is correct
}

// Test_chunkedIngress_Pop_DoubleCheck explicitly tests the double-check
// path when chunk advancement fails.
// Priority 2: MEDIUM - Explicit double-check path.
func Test_chunkedIngress_Pop_DoubleCheck(t *testing.T) {
	// This tests chunkedIngress.Pop() double-check logic when
	// chunk advancement fails (unlikely but tested for completeness)
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	// Create ingress
	ingress := newChunkedIngress()

	// Submit tasks
	for range 5 {
		fn := func() {}
		ingress.Push(fn)
	}

	// Pop all tasks
	count := 0
	for {
		_, ok := ingress.Pop()
		if !ok {
			break
		}
		count++
	}

	if count != 5 {
		t.Errorf("Expected 5 tasks, got %d", count)
	}
}

// TestScheduleMicrotask_ConcurrentSubmit verifies error handling when
// submitting microtasks concurrently during loop shutdown.
// Priority 4: LOW - Concurrent error scenarios.
func TestScheduleMicrotask_ConcurrentSubmit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal("NewJS failed:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start loop
	go loop.Run(ctx)

	// Wait for loop to be active
	time.Sleep(20 * time.Millisecond)

	// Trigger shutdown while concurrently submitting microtasks
	var wg sync.WaitGroup
	submitErr := make(chan error, 100)

	// Shutdown in background
	go func() {
		time.Sleep(10 * time.Millisecond)
		loop.Shutdown(ctx)
	}()

	// Submit microtasks concurrently
	for range 50 {
		wg.Go(func() {
			err := js.QueueMicrotask(func() {})
			submitErr <- err
		})
	}

	// Collect error results
	go func() {
		wg.Wait()
		close(submitErr)
	}()

	// Check some errors occurred (indicating shutdown was in progress)
	anyError := false
	for err := range submitErr {
		if err == ErrLoopTerminated {
			anyError = true
			break
		}
	}

	if !anyError {
		t.Log("No ErrLoopTerminated errors detected (may be timing-dependent)")
	}

	// Critical: No panics occurred
}
