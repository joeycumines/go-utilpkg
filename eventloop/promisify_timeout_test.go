package eventloop

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Tests for PromisifyWithTimeout and PromisifyWithDeadline

// TestPromisifyWithTimeout_Success tests successful completion before timeout.
func TestPromisifyWithTimeout_Success(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	p := loop.PromisifyWithTimeout(ctx, 1*time.Second, func(ctx context.Context) (any, error) {
		return "success", nil
	})

	select {
	case result := <-p.ToChannel():
		if result != "success" {
			t.Errorf("Expected 'success', got %v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for promise")
	}
}

// TestPromisifyWithTimeout_Timeout tests rejection on timeout.
func TestPromisifyWithTimeout_Timeout(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	p := loop.PromisifyWithTimeout(ctx, 50*time.Millisecond, func(ctx context.Context) (any, error) {
		// Wait for context cancellation (timeout)
		<-ctx.Done()
		return nil, ctx.Err()
	})

	select {
	case result := <-p.ToChannel():
		err, ok := result.(error)
		if !ok || !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected DeadlineExceeded, got %v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for promise rejection")
	}

	// Verify promise state
	if p.State() != Rejected {
		t.Errorf("Expected Rejected state, got %v", p.State())
	}
}

// TestPromisifyWithTimeout_FunctionError tests function returning error.
func TestPromisifyWithTimeout_FunctionError(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	testErr := errors.New("test error")

	p := loop.PromisifyWithTimeout(ctx, 1*time.Second, func(ctx context.Context) (any, error) {
		return nil, testErr
	})

	select {
	case result := <-p.ToChannel():
		err, ok := result.(error)
		if !ok || !errors.Is(err, testErr) {
			t.Errorf("Expected testErr, got %v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for promise rejection")
	}
}

// TestPromisifyWithTimeout_ParentContextCancelled tests parent context cancellation.
func TestPromisifyWithTimeout_ParentContextCancelled(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	// Create a parent context that we will cancel
	parentCtx, parentCancel := context.WithCancel(ctx)

	p := loop.PromisifyWithTimeout(parentCtx, 10*time.Second, func(ctx context.Context) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	// Cancel parent context
	parentCancel()

	select {
	case result := <-p.ToChannel():
		err, ok := result.(error)
		if !ok || !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled, got %v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for promise rejection")
	}
}

// TestPromisifyWithTimeout_Panic tests panic in function.
func TestPromisifyWithTimeout_Panic(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	p := loop.PromisifyWithTimeout(ctx, 1*time.Second, func(ctx context.Context) (any, error) {
		panic("test panic")
	})

	select {
	case result := <-p.ToChannel():
		panicErr, ok := result.(PanicError)
		if !ok {
			t.Errorf("Expected PanicError, got %T: %v", result, result)
		} else if panicErr.Value != "test panic" {
			t.Errorf("Expected 'test panic', got %v", panicErr.Value)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for promise rejection")
	}
}

// === PromisifyWithDeadline tests ===

// TestPromisifyWithDeadline_Success tests successful completion before deadline.
func TestPromisifyWithDeadline_Success(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	deadline := time.Now().Add(1 * time.Second)
	p := loop.PromisifyWithDeadline(ctx, deadline, func(ctx context.Context) (any, error) {
		return "success", nil
	})

	select {
	case result := <-p.ToChannel():
		if result != "success" {
			t.Errorf("Expected 'success', got %v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for promise")
	}
}

// TestPromisifyWithDeadline_DeadlineExceeded tests rejection on deadline.
func TestPromisifyWithDeadline_DeadlineExceeded(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	deadline := time.Now().Add(50 * time.Millisecond)
	p := loop.PromisifyWithDeadline(ctx, deadline, func(ctx context.Context) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	select {
	case result := <-p.ToChannel():
		err, ok := result.(error)
		if !ok || !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected DeadlineExceeded, got %v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for promise rejection")
	}
}

// TestPromisifyWithDeadline_PastDeadline tests immediate rejection for past deadline.
func TestPromisifyWithDeadline_PastDeadline(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	// Deadline already passed
	deadline := time.Now().Add(-1 * time.Second)
	p := loop.PromisifyWithDeadline(ctx, deadline, func(ctx context.Context) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	select {
	case result := <-p.ToChannel():
		err, ok := result.(error)
		if !ok || !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected DeadlineExceeded, got %v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for promise rejection")
	}
}

// TestPromisifyWithDeadline_FunctionError tests function returning error.
func TestPromisifyWithDeadline_FunctionError(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	testErr := errors.New("test error")

	deadline := time.Now().Add(1 * time.Second)
	p := loop.PromisifyWithDeadline(ctx, deadline, func(ctx context.Context) (any, error) {
		return nil, testErr
	})

	select {
	case result := <-p.ToChannel():
		err, ok := result.(error)
		if !ok || !errors.Is(err, testErr) {
			t.Errorf("Expected testErr, got %v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for promise rejection")
	}
}

// TestPromisifyWithDeadline_Panic tests panic in function.
func TestPromisifyWithDeadline_Panic(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	deadline := time.Now().Add(1 * time.Second)
	p := loop.PromisifyWithDeadline(ctx, deadline, func(ctx context.Context) (any, error) {
		panic("test panic")
	})

	select {
	case result := <-p.ToChannel():
		panicErr, ok := result.(PanicError)
		if !ok {
			t.Errorf("Expected PanicError, got %T: %v", result, result)
		} else if panicErr.Value != "test panic" {
			t.Errorf("Expected 'test panic', got %v", panicErr.Value)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for promise rejection")
	}
}

// TestPromisifyWithTimeout_ZeroTimeout tests immediate timeout with zero duration.
func TestPromisifyWithTimeout_ZeroTimeout(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	p := loop.PromisifyWithTimeout(ctx, 0, func(ctx context.Context) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	select {
	case result := <-p.ToChannel():
		err, ok := result.(error)
		if !ok || !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected DeadlineExceeded, got %v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for promise rejection")
	}
}

// TestPromisifyWithTimeout_LongRunningSuccess tests long operation that completes before timeout.
func TestPromisifyWithTimeout_LongRunningSuccess(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	p := loop.PromisifyWithTimeout(ctx, 2*time.Second, func(ctx context.Context) (any, error) {
		// Simulate work that takes some time but completes before timeout
		time.Sleep(50 * time.Millisecond)
		return 42, nil
	})

	select {
	case result := <-p.ToChannel():
		if result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for promise")
	}

	// Verify state
	if p.State() != Fulfilled {
		t.Errorf("Expected Fulfilled state, got %v", p.State())
	}
}

// TestPromisifyWithDeadline_ContextRespected tests that the function respects context cancellation.
func TestPromisifyWithDeadline_ContextRespected(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	receivedContext := make(chan bool, 1)

	deadline := time.Now().Add(50 * time.Millisecond)
	_ = loop.PromisifyWithDeadline(ctx, deadline, func(ctx context.Context) (any, error) {
		// Verify context has deadline
		if _, ok := ctx.Deadline(); ok {
			receivedContext <- true
		} else {
			receivedContext <- false
		}
		<-ctx.Done()
		return nil, ctx.Err()
	})

	select {
	case hasDeadline := <-receivedContext:
		if !hasDeadline {
			t.Error("Context did not have deadline set")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for context check")
	}
}
