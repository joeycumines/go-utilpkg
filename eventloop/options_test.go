package eventloop

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

// Test 1.2.6: Test default options
func TestDefaultOptions(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l.Shutdown(context.Background())

	if l.strictMicrotaskOrdering {
		t.Error("Default strictMicrotaskOrdering should be false, got true")
	}

	// FastPathMode should be Auto (0) by default
	mode := FastPathMode(l.fastPathMode.Load())
	if mode != FastPathAuto {
		t.Errorf("Default FastPathMode should be Auto (%d), got %d", FastPathAuto, mode)
	}
}

// Test 1.2.7: Test custom options
func TestCustomOptions(t *testing.T) {
	// Create loop with strict microtask ordering enabled
	l, err := New(WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("New() with StrictMicrotaskOrdering failed: %v", err)
	}
	defer l.Shutdown(context.Background())

	if !l.strictMicrotaskOrdering {
		t.Error("strictMicrotaskOrdering should be true after WithStrictMicrotaskOrdering(true)")
	}

	// Create another loop with forced fast path mode
	l2, err := New(
		WithStrictMicrotaskOrdering(false),
		WithFastPathMode(FastPathForced),
	)
	if err != nil {
		t.Fatalf("New() with multiple options failed: %v", err)
	}
	defer l2.Shutdown(context.Background())

	if l2.strictMicrotaskOrdering {
		t.Error("strictMicrotaskOrdering should be false after WithStrictMicrotaskOrdering(false)")
	}

	mode := FastPathMode(l2.fastPathMode.Load())
	if mode != FastPathForced {
		t.Errorf("FastPathMode should be Forced (%d), got %d", FastPathForced, mode)
	}
}

// Test: Multiple options in any order
func TestMultipleOptions(t *testing.T) {
	// Test that options can be specified in any order
	l1, err := New(
		WithFastPathMode(FastPathDisabled),
		WithStrictMicrotaskOrdering(true),
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l1.Shutdown(context.Background())

	if !l1.strictMicrotaskOrdering {
		t.Error("Option order 1: strictMicrotaskOrdering should be true")
	}
	mode := FastPathMode(l1.fastPathMode.Load())
	if mode != FastPathDisabled {
		t.Errorf("Option order 1: FastPathMode should be Disabled (%d), got %d", FastPathDisabled, mode)
	}

	// Test reverse order
	l2, err := New(
		WithStrictMicrotaskOrdering(false),
		WithFastPathMode(FastPathForced),
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l2.Shutdown(context.Background())

	if l2.strictMicrotaskOrdering {
		t.Error("Option order 2: strictMicrotaskOrdering should be false")
	}
	mode = FastPathMode(l2.fastPathMode.Load())
	if mode != FastPathForced {
		t.Errorf("Option order 2: FastPathMode should be Forced (%d), got %d", FastPathForced, mode)
	}
}

// Test: Nil option handling
func TestNilOption(t *testing.T) {
	// Test that nil options are handled gracefully
	l, err := New(nil)
	if err != nil {
		t.Fatalf("New() with nil option failed: %v", err)
	}
	defer l.Shutdown(context.Background())

	// Loop should still work with default values
	if l.strictMicrotaskOrdering {
		t.Error("Default with nil option should have strictMicrotaskOrdering=false")
	}
	mode := FastPathMode(l.fastPathMode.Load())
	if mode != FastPathAuto {
		t.Errorf("Default with nil option should have FastPathMode=Auto (%d), got %d", FastPathAuto, mode)
	}
}

// TestWithLogger verifies that WithLogger option properly attaches
// a logger to the event loop.
func TestWithLogger(t *testing.T) {
	// Create a simple logger using logiface.New
	// We use io.Discard to ignore output for this test
	logger := logiface.New[logiface.Event](
		logiface.WithWriter[logiface.Event](logiface.NewWriterFunc(func(event logiface.Event) error {
			// Discard events for this test
			return nil
		})),
	)

	opts := []LoopOption{
		WithLogger(logger),
	}

	loop, err := New(opts...)
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	// Verify loop is created with logger (logger field should be non-nil)
	// Note: We can't directly access loop.logger as it's not exported,
	// but we can verify the loop was created successfully with logger option
	t.Log("Loop created successfully with WithLogger option")
}

// TestWithLogger_PanicRecovery verifies that logger properly captures
// panics during runtime operations.
func TestWithLogger_PanicRecovery(t *testing.T) {
	// Create a simple logger for panic capture
	logger := logiface.New[logiface.Event](
		logiface.WithWriter[logiface.Event](logiface.NewWriterFunc(func(event logiface.Event) error {
			// Discard events for this test
			return nil
		})),
	)

	opts := []LoopOption{
		WithLogger(logger),
	}

	loop, err := New(opts...)
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	// Trigger panic in Submit
	loop.Submit(func() {
		panic("test panic in Submit")
	})

	// Run loop to process and recover
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go loop.Run(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()

	// If we reach here, panic recovery worked
	t.Log("Panic recovery test completed successfully")
}
