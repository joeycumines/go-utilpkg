//go:build linux || darwin

package alternatetwo

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = loop.Shutdown(ctx)
	}()

	if loop.State() != StateAwake {
		t.Errorf("Expected state Awake, got %v", loop.State())
	}
}

func TestRunShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Run Loop in a goroutine since Run() is blocking
	runDone := make(chan error, 1)
	ctx := context.Background()
	go func() {
		runDone <- loop.Run(ctx)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	state := loop.State()
	if state != StateRunning && state != StateSleeping {
		t.Errorf("Expected Running or Sleeping, got %v", state)
	}

	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := loop.Shutdown(stopCtx); err != nil {
		t.Fatalf("Shutdown() failed: %v", err)
	}

	// Wait for Run() to return
	if err := <-runDone; err != nil && err != context.Canceled {
		t.Fatalf("Run() returned error: %v", err)
	}

	if loop.State() != StateTerminated {
		t.Errorf("Expected state Terminated, got %v", loop.State())
	}
}

func TestSubmit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	go loop.Run(ctx)

	var executed atomic.Bool
	done := make(chan struct{})

	err = loop.Submit(func() {
		executed.Store(true)
		close(done)
	})
	if err != nil {
		t.Fatalf("Submit() failed: %v", err)
	}

	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("Task not executed within timeout")
	}

	if !executed.Load() {
		t.Error("Task was not executed")
	}

	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
}

func TestSubmitInternal(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	go loop.Run(ctx)

	var executed atomic.Bool
	done := make(chan struct{})

	err = loop.SubmitInternal(func() {
		executed.Store(true)
		close(done)
	})
	if err != nil {
		t.Fatalf("SubmitInternal() failed: %v", err)
	}

	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("Internal task not executed within timeout")
	}

	if !executed.Load() {
		t.Error("Internal task was not executed")
	}

	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
}

func TestMicrotask(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	go loop.Run(ctx)

	var executed atomic.Bool
	done := make(chan struct{})

	// Schedule a microtask from within the loop
	err = loop.Submit(func() {
		_ = loop.ScheduleMicrotask(func() {
			executed.Store(true)
			close(done)
		})
	})
	if err != nil {
		t.Fatalf("Submit() failed: %v", err)
	}

	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("Microtask not executed within timeout")
	}

	if !executed.Load() {
		t.Error("Microtask was not executed")
	}

	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
}

func TestConcurrentSubmit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	go loop.Run(ctx)

	const numTasks = 1000
	var executed atomic.Int64
	var wg sync.WaitGroup
	wg.Add(numTasks)

	for i := 0; i < numTasks; i++ {
		go func() {
			err := loop.Submit(func() {
				executed.Add(1)
				wg.Done()
			})
			if err != nil {
				wg.Done() // Count as done even if failed
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatalf("Only %d/%d tasks executed within timeout", executed.Load(), numTasks)
	}

	if executed.Load() != numTasks {
		t.Errorf("Expected %d tasks executed, got %d", numTasks, executed.Load())
	}

	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
}

func TestDoubleStart(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	go loop.Run(ctx)

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	// Second start attempt (from different goroutine) should fail with ErrLoopAlreadyRunning
	err = loop.Run(ctx)
	if err != ErrLoopAlreadyRunning {
		t.Errorf("Expected ErrLoopAlreadyRunning, got %v", err)
	}

	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
}

func TestDoubleShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	go loop.Run(ctx)

	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	if err := loop.Shutdown(stopCtx); err != nil {
		t.Fatalf("First Shutdown() failed: %v", err)
	}

	// Second shutdown should not error (idempotent via sync.Once)
	if err := loop.Shutdown(stopCtx); err != nil && err != ErrLoopTerminated {
		t.Errorf("Second Shutdown() failed unexpectedly: %v", err)
	}
}

func TestSubmitAfterShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	go loop.Run(ctx)

	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)

	// Submit after shutdown should fail
	err = loop.Submit(func() {})
	if err != ErrLoopTerminated {
		t.Errorf("Expected ErrLoopTerminated, got %v", err)
	}
}

func TestFastState(t *testing.T) {
	s := NewFastState()

	if s.Load() != StateAwake {
		t.Errorf("Expected StateAwake, got %v", s.Load())
	}

	if !s.TryTransition(StateAwake, StateRunning) {
		t.Error("Transition Awake -> Running should succeed")
	}

	if s.Load() != StateRunning {
		t.Errorf("Expected StateRunning, got %v", s.Load())
	}

	// Failed transition (wrong from state)
	if s.TryTransition(StateAwake, StateSleeping) {
		t.Error("Transition from wrong state should fail")
	}
}

func TestLockFreeIngress(t *testing.T) {
	q := NewLockFreeIngress()

	if !q.IsEmpty() {
		t.Error("New queue should be empty")
	}

	// Push some tasks
	for i := 0; i < 100; i++ {
		q.Push(func() {})
	}

	if q.Length() != 100 {
		t.Errorf("Expected length 100, got %d", q.Length())
	}

	// Pop all tasks
	for i := 0; i < 100; i++ {
		_, ok := q.Pop()
		if !ok {
			t.Errorf("Pop failed at iteration %d", i)
		}
	}

	if !q.IsEmpty() {
		t.Error("Queue should be empty after popping all")
	}
}

func TestMicrotaskRing(t *testing.T) {
	r := NewMicrotaskRing()

	if !r.IsEmpty() {
		t.Error("New ring should be empty")
	}

	// Push some microtasks
	for i := 0; i < 100; i++ {
		if !r.Push(func() {}) {
			t.Errorf("Push failed at iteration %d", i)
		}
	}

	if r.Length() != 100 {
		t.Errorf("Expected length 100, got %d", r.Length())
	}

	// Pop all
	for i := 0; i < 100; i++ {
		fn := r.Pop()
		if fn == nil {
			t.Errorf("Pop returned nil at iteration %d", i)
		}
	}

	if !r.IsEmpty() {
		t.Error("Ring should be empty after popping all")
	}
}

func TestChunkMinimalClearing(t *testing.T) {
	// This test verifies the minimal clearing behavior
	c := newChunk()

	// Fill some slots
	for i := 0; i < 50; i++ {
		c.tasks[i] = Task{Fn: func() {}}
		c.pos++
	}

	// Return with minimal clearing
	returnChunkFast(c)

	// Verify cleared slots
	for i := 0; i < 50; i++ {
		if c.tasks[i].Fn != nil {
			t.Errorf("Slot %d not cleared", i)
		}
	}

	// Verify cursors reset
	if c.pos != 0 || c.readPos != 0 {
		t.Error("Cursors not reset")
	}
}

func TestBlockingRun(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Test that Run blocks until terminated
	runDone := make(chan error, 1)
	ctx := context.Background()

	go func() {
		runDone <- loop.Run(ctx)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	// Run should not have returned yet
	select {
	case <-runDone:
		t.Fatal("Run() returned early, should be blocking")
	default:
		// OK - Run is still blocking
	}

	// Shutdown the loop
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := loop.Shutdown(stopCtx); err != nil {
		t.Fatalf("Shutdown() failed: %v", err)
	}

	// Now Run should have returned
	select {
	case err := <-runDone:
		if err != nil && err != context.Canceled {
			t.Errorf("Run() returned unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not return after Shutdown")
	}
}

func TestCloseImmediate(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Close on unstarted loop should succeed and set state to Terminated
	if err := loop.Close(); err != nil {
		t.Fatalf("First Close() failed: %v", err)
	}

	if loop.State() != StateTerminated {
		t.Errorf("Expected state Terminated, got %v", loop.State())
	}

	// Second Close should return ErrLoopTerminated
	err = loop.Close()
	if err != ErrLoopTerminated {
		t.Errorf("Expected ErrLoopTerminated, got %v", err)
	}
}

func TestRunWithContextCancel(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Test that Run exits when context is canceled
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)

	go func() {
		runDone <- loop.Run(ctx)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	// Cancel the context
	cancel()

	// Run should exit
	select {
	case err := <-runDone:
		if err != nil && err != context.Canceled {
			t.Logf("Run() returned: %v (expected context.Canceled or nil)", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not exit after context cancel")
	}

	// State should be Terminating or Terminated after context cancel
	// Since Run() returns early on context cancel, state may not be Terminated yet
	state := loop.State()
	if state != StateTerminated && state != StateTerminating {
		t.Errorf("Expected state Terminating or Terminated, got %v", state)
	}
}

func BenchmarkSubmit(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	go loop.Run(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.Submit(func() {})
	}
	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
}

func BenchmarkLockFreeIngress_Push(b *testing.B) {
	q := NewLockFreeIngress()
	fn := func() {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(fn)
	}
}

func BenchmarkLockFreeIngress_PushPop(b *testing.B) {
	q := NewLockFreeIngress()
	fn := func() {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(fn)
		q.Pop()
	}
}

func BenchmarkMicrotaskRing_PushPop(b *testing.B) {
	r := NewMicrotaskRing()
	fn := func() {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Push(fn)
		r.Pop()
	}
}
