package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLoop_SubmitMultiple tests submitting multiple tasks
func TestLoop_SubmitMultiple(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	var count int32

	// Submit 100 tasks
	for i := 0; i < 100; i++ {
		loop.Submit(func() {
			atomic.AddInt32(&count, 1)
		})
	}

	// Wait for completion
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&count) != 100 {
		t.Errorf("Expected 100 tasks, got: %d", atomic.LoadInt32(&count))
	}

	cancel()
	loop.Shutdown(context.Background())
	<-done
}

// TestLoop_SubmitAfterShutdown tests submitting after shutdown
func TestLoop_SubmitAfterShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Start loop
	time.Sleep(10 * time.Millisecond)

	// Shutdown
	cancel()
	loop.Shutdown(context.Background())
	<-done

	// Submit after shutdown should fail
	err = loop.Submit(func() {})
	if err != ErrLoopTerminated {
		t.Errorf("Expected ErrLoopTerminated, got: %v", err)
	}
}

// TestLoop_SubmitInternal tests SubmitInternal method
func TestLoop_SubmitInternal(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// SubmitInternal should work
	err = loop.SubmitInternal(func() {})
	if err != nil {
		t.Errorf("SubmitInternal failed: %v", err)
	}

	loop.Shutdown(context.Background())
}

// TestLoop_SubmitInternalAfterShutdown tests SubmitInternal after shutdown
func TestLoop_SubmitInternalAfterShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Start loop
	time.Sleep(10 * time.Millisecond)

	// Shutdown
	cancel()
	loop.Shutdown(context.Background())
	<-done

	// SubmitInternal after shutdown should fail
	err = loop.SubmitInternal(func() {})
	if err != ErrLoopTerminated {
		t.Errorf("Expected ErrLoopTerminated, got: %v", err)
	}
}

// TestLoop_Microtask tests microtask scheduling
func TestLoop_Microtask(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	var executed int32

	// Schedule microtasks
	for i := 0; i < 10; i++ {
		loop.ScheduleMicrotask(func() {
			atomic.AddInt32(&executed, 1)
		})
	}

	// Wait for execution
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&executed) != 10 {
		t.Errorf("Expected 10 microtasks, got: %d", atomic.LoadInt32(&executed))
	}

	cancel()
	loop.Shutdown(context.Background())
	<-done
}

// TestLoop_MicrotaskWithDelay tests microtask scheduling with delay
func TestLoop_MicrotaskWithDelay(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	var executed int32
	started := make(chan struct{})

	// Schedule microtask
	loop.ScheduleMicrotask(func() {
		atomic.AddInt32(&executed, 1)
		close(started)
	})

	// Wait for execution
	select {
	case <-started:
	case <-time.After(100 * time.Millisecond):
		t.Error("Microtask did not execute")
	}

	if executed != 1 {
		t.Errorf("Expected 1 microtask, got: %d", executed)
	}

	cancel()
	loop.Shutdown(context.Background())
	<-done
}

// TestLoop_RunContextCancel tests running loop with context cancel
func TestLoop_RunContextCancel(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	submitted := int32(0)

	go func() {
		loop.Run(ctx)
	}()

	// Submit tasks
	for i := 0; i < 10; i++ {
		loop.Submit(func() {
			atomic.AddInt32(&submitted, 1)
		})
	}

	// Wait for some tasks
	time.Sleep(50 * time.Millisecond)

	loop.Shutdown(context.Background())

	// Some tasks may have been submitted
	_ = submitted
}

// TestLoop_ConcurrentSubmit tests concurrent submission
func TestLoop_ConcurrentSubmit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	var count int32

	// Concurrent submission
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			loop.Submit(func() {
				atomic.AddInt32(&count, 1)
			})
		}()
	}

	wg.Wait()

	// Wait for completion
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&count) != 100 {
		t.Errorf("Expected 100 tasks, got: %d", atomic.LoadInt32(&count))
	}

	cancel()
	loop.Shutdown(context.Background())
	<-done
}

// TestLoop_DoubleShutdown tests double shutdown
func TestLoop_DoubleShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Start loop
	time.Sleep(10 * time.Millisecond)

	// Double shutdown should be safe
	loop.Shutdown(context.Background())
	loop.Shutdown(context.Background())

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("Loop did not shutdown")
	}
}

// TestLoop_SubmitNilTask tests submitting nil task
func TestLoop_SubmitNilTask(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// Submit nil task should not panic
	err = loop.Submit(nil)
	if err != nil {
		t.Logf("Submit nil returned error: %v", err)
	}

	loop.Shutdown(context.Background())
}

// TestLoop_SubmitInternalNilTask tests submitting nil task via SubmitInternal
func TestLoop_SubmitInternalNilTask(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// SubmitInternal nil task should not panic
	err = loop.SubmitInternal(nil)
	if err != nil {
		t.Logf("SubmitInternal nil returned error: %v", err)
	}

	loop.Shutdown(context.Background())
}

// TestLoop_Metrics tests loop metrics
func TestLoop_Metrics(t *testing.T) {
	loop, err := New(WithMetrics(true))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	// Submit some tasks
	for i := 0; i < 10; i++ {
		loop.Submit(func() {})
	}

	// Schedule some microtasks
	for i := 0; i < 5; i++ {
		loop.ScheduleMicrotask(func() {})
	}

	// Get metrics
	metrics := loop.Metrics()
	if metrics == nil {
		t.Error("Metrics should not be nil")
	}

	cancel()
	loop.Shutdown(context.Background())
	<-done
}

// TestLoop_StateTransitions tests loop state transitions
func TestLoop_StateTransitions(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Let loop start
	time.Sleep(10 * time.Millisecond)

	// State should be running or sleeping
	state := loop.state.Load()
	if state != StateRunning && state != StateSleeping {
		t.Errorf("Expected Running or Sleeping, got: %v", state)
	}

	// Cancel and wait
	cancel()
	loop.Shutdown(context.Background())

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("Loop did not shutdown")
	}
}

// TestLoop_CloseImmediate tests immediate close
func TestLoop_CloseImmediate(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// Close immediately should not panic
	loop.Shutdown(context.Background())
}

// TestLoop_SubmitAfterTerminated tests submitting after terminated
func TestLoop_SubmitAfterTerminated(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Start loop
	time.Sleep(10 * time.Millisecond)

	// Terminate
	cancel()
	loop.Shutdown(context.Background())

	<-done

	// Multiple submits after termination
	for i := 0; i < 10; i++ {
		err = loop.Submit(func() {})
		if err != ErrLoopTerminated {
			t.Errorf("Expected ErrLoopTerminated, got: %v", err)
		}
	}
}

// TestLoop_FastPathSubmits tests fast path submit optimization
func TestLoop_FastPathSubmits(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// Submit many small tasks to trigger fast path
	for i := 0; i < 1000; i++ {
		loop.Submit(func() {
			// Empty task
		})
	}

	// Wait for completion
	time.Sleep(100 * time.Millisecond)

	loop.Shutdown(context.Background())
}

// TestLoop_MixedOperations tests mixed submit and microtask operations
func TestLoop_MixedOperations(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	var submitCount int32
	var microtaskCount int32

	// Mixed operations
	for i := 0; i < 50; i++ {
		loop.Submit(func() {
			atomic.AddInt32(&submitCount, 1)
		})
		loop.ScheduleMicrotask(func() {
			atomic.AddInt32(&microtaskCount, 1)
		})
	}

	// Wait for completion
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&submitCount) != 50 {
		t.Errorf("Expected 50 submits, got: %d", atomic.LoadInt32(&submitCount))
	}
	if atomic.LoadInt32(&microtaskCount) != 50 {
		t.Errorf("Expected 50 microtasks, got: %d", atomic.LoadInt32(&microtaskCount))
	}

	cancel()
	loop.Shutdown(context.Background())
	<-done
}
