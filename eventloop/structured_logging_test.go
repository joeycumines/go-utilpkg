package eventloop

import (
	"context"
	"testing"
	"time"
)

// TestStructuredLogging_TaskPanic tests that task panics are handled gracefully.
func TestStructuredLogging_TaskPanic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = loop.Run(context.Background())
		close(done)
	}()

	// Submit a panicking task
	if err := loop.Submit(func() {
		panic("test panic")
	}); err != nil {
		t.Fatalf("failed to submit: %v", err)
	}

	// Give time for panic to be handled
	time.Sleep(50 * time.Millisecond)

	loop.Shutdown(context.Background())
	<-done

	// Test passes if loop didn't crash from the panic
}

// TestStructuredLogging_OnOverloadPanic tests that OnOverload panics are handled.
func TestStructuredLogging_OnOverloadPanic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	// Set OnOverload callback that panics
	loop.OnOverload = func(err error) {
		panic("overload panic")
	}

	done := make(chan struct{})
	go func() {
		_ = loop.Run(context.Background())
		close(done)
	}()

	// Flood the loop to trigger overload
	for i := 0; i < 1000; i++ {
		_ = loop.Submit(func() {
			// Slow task
			time.Sleep(time.Millisecond)
		})
	}

	// Give time for overload to be triggered
	time.Sleep(100 * time.Millisecond)

	loop.Shutdown(context.Background())
	<-done

	// Test passes if loop didn't crash from the panic
}

// TestStructuredLogging_NoLogger tests backward compatibility with no logger.
func TestStructuredLogging_NoLogger(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = loop.Run(context.Background())
		close(done)
	}()

	// Submit a panicking task - should use log.Printf fallback
	if err := loop.Submit(func() {
		panic("no logger panic")
	}); err != nil {
		t.Fatalf("failed to submit: %v", err)
	}

	// Give time for panic to be handled
	time.Sleep(50 * time.Millisecond)

	loop.Shutdown(context.Background())
	<-done

	// Test passes if no crash occurred
}

// TestStructuredLogging_NilLogger tests that WithLogger(nil) is accepted.
func TestStructuredLogging_NilLogger(t *testing.T) {
	loop, err := New(WithLogger(nil))
	if err != nil {
		t.Fatalf("failed to create loop with nil logger: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = loop.Run(context.Background())
		close(done)
	}()

	// Submit a panicking task - should use log.Printf fallback
	if err := loop.Submit(func() {
		panic("nil logger panic")
	}); err != nil {
		t.Fatalf("failed to submit: %v", err)
	}

	// Give time for panic to be handled
	time.Sleep(50 * time.Millisecond)

	loop.Shutdown(context.Background())
	<-done

	// Test passes if no crash occurred
}

// TestStructuredLogging_MicrotaskPanic tests that microtask panics are handled.
func TestStructuredLogging_MicrotaskPanic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = loop.Run(context.Background())
		close(done)
	}()

	// Submit a task that schedules a panicking microtask
	if err := loop.Submit(func() {
		loop.ScheduleMicrotask(func() {
			panic("microtask panic")
		})
	}); err != nil {
		t.Fatalf("failed to submit: %v", err)
	}

	// Give time for panic to be handled
	time.Sleep(50 * time.Millisecond)

	loop.Shutdown(context.Background())
	<-done

	// Test passes if loop didn't crash from the microtask panic
}

// TestStructuredLogging_LogError tests the logError helper directly.
func TestStructuredLogging_LogError(t *testing.T) {
	t.Run("without logger", func(t *testing.T) {
		loop, err := New()
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		// Call logError directly - should not panic
		loop.logError("test message", "panic value")
	})

	t.Run("with nil logger", func(t *testing.T) {
		loop, err := New(WithLogger(nil))
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		// Call logError directly - should not panic
		loop.logError("test message", "panic value")
	})
}

// TestStructuredLogging_LogCritical tests the logCritical helper directly.
func TestStructuredLogging_LogCritical(t *testing.T) {
	t.Run("without logger", func(t *testing.T) {
		loop, err := New()
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		// Call logCritical directly - should not panic
		loop.logCritical("critical test", ErrLoopTerminated)
	})

	t.Run("with nil logger", func(t *testing.T) {
		loop, err := New(WithLogger(nil))
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		// Call logCritical directly - should not panic
		loop.logCritical("critical test", ErrLoopTerminated)
	})
}
