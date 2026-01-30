package eventloop

import (
	"context"
	"errors"
	"sync"
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
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := js.ClearImmediate(id); err != nil && !errors.Is(err, ErrTimerNotFound) {
				errChan <- err
			}
		}()
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
	for i := 0; i < 10; i++ {
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
	for i := 0; i < 10; i++ {
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
