package eventloop

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

// TestPromisify_ContextCancellation verifies that Promisify respects context
// cancellation and the promise is rejected with context.Canceled.
func TestPromisify_ContextCancellation(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		if err := loop.Run(ctx); err != nil {
			t.Errorf("Run() unexpected error: %v", err)
		}
		close(runDone)
	}()

	taskCtx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{})
	promise := loop.Promisify(taskCtx, func(ctx context.Context) (Result, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})

	<-started
	cancel()

	ch := promise.ToChannel()
	select {
	case result := <-ch:
		err, ok := result.(error)
		if !ok {
			t.Fatalf("Expected error result, got: %v", result)
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Expected context.Canceled, got: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Promise never resolved after cancellation")
	}

	loop.Shutdown(context.Background())
	<-runDone
}

// does not leak the Promisify worker goroutine.
func TestPromisify_Cancellation_GoroutineLeak(t *testing.T) {
	l, _ := New()
	go func() {
		if err := l.Run(context.Background()); err != nil {
			t.Errorf("Run() unexpected error: %v", err)
		}
	}()
	defer l.Shutdown(context.Background())

	runtime.GC()
	startRoutines := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())

	l.Promisify(ctx, func(innerCtx context.Context) (Result, error) {
		<-innerCtx.Done()
		return nil, innerCtx.Err()
	})

	cancel()

	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	endRoutines := runtime.NumGoroutine()

	if endRoutines > startRoutines+1 {
		t.Fatalf("Goroutine Leak! Started with %d, ended with %d. "+
			"Promisify worker failed to exit on context cancellation.",
			startRoutines, endRoutines)
	}
}
