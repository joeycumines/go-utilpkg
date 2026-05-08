package alternatetwo

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), alternateTwoTestTimeout)
		defer cancel()
		if err := loop.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown() failed: %v", err)
		}
	})

	if loop.State() != StateAwake {
		t.Errorf("Expected state Awake, got %v", loop.State())
	}
}

func TestRunShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateTwoTestLoop(t, loop)

	state := loop.State()
	if state != StateRunning && state != StateSleeping {
		t.Errorf("Expected Running or Sleeping, got %v", state)
	}

	harness.shutdown(t)

	if loop.State() != StateTerminated {
		t.Errorf("Expected state Terminated, got %v", loop.State())
	}
}

func TestSubmit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateTwoTestLoop(t, loop)

	var executed atomic.Bool
	done := make(chan struct{})

	err = loop.Submit(func() {
		executed.Store(true)
		close(done)
	})
	if err != nil {
		t.Fatalf("Submit() failed: %v", err)
	}

	waitAlternateTwoSignal(t, done, "submitted task")

	if !executed.Load() {
		t.Error("Task was not executed")
	}

	harness.shutdown(t)
}

func TestSubmitInternal(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateTwoTestLoop(t, loop)

	var executed atomic.Bool
	done := make(chan struct{})

	err = loop.SubmitInternal(func() {
		executed.Store(true)
		close(done)
	})
	if err != nil {
		t.Fatalf("SubmitInternal() failed: %v", err)
	}

	waitAlternateTwoSignal(t, done, "internal task")

	if !executed.Load() {
		t.Error("Internal task was not executed")
	}

	harness.shutdown(t)
}

func TestMicrotask(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateTwoTestLoop(t, loop)

	var executed atomic.Bool
	done := make(chan struct{})

	// Schedule a microtask from within the loop
	scheduleResult := make(chan error, 1)
	err = loop.Submit(func() {
		scheduleResult <- loop.ScheduleMicrotask(func() {
			executed.Store(true)
			close(done)
		})
	})
	if err != nil {
		t.Fatalf("Submit() failed: %v", err)
	}

	select {
	case scheduleErr := <-scheduleResult:
		if scheduleErr != nil {
			t.Fatalf("ScheduleMicrotask() failed: %v", scheduleErr)
		}
	case <-time.After(alternateTwoTestTimeout):
		t.Fatal("ScheduleMicrotask() did not return")
	}
	waitAlternateTwoSignal(t, done, "microtask")

	if !executed.Load() {
		t.Error("Microtask was not executed")
	}

	harness.shutdown(t)
}

func TestConcurrentSubmit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateTwoTestLoop(t, loop)

	const numTasks = 1000
	var executed atomic.Int64
	results := make(chan error, numTasks)
	callbacks := make(chan struct{}, numTasks)

	for range numTasks {
		go func() {
			results <- loop.Submit(func() {
				executed.Add(1)
				callbacks <- struct{}{}
			})
		}()
	}
	deadline := time.NewTimer(alternateTwoTestTimeout)
	defer deadline.Stop()
	for range numTasks {
		select {
		case submitErr := <-results:
			if submitErr != nil {
				t.Fatalf("concurrent Submit() failed: %v", submitErr)
			}
		case <-deadline.C:
			t.Fatalf("only %d/%d submissions returned", executed.Load(), numTasks)
		}
	}
	for range numTasks {
		waitAlternateTwoSignal(t, callbacks, "concurrent callback")
	}

	if executed.Load() != numTasks {
		t.Errorf("Expected %d tasks executed, got %d", numTasks, executed.Load())
	}

	harness.shutdown(t)
}

func TestDoubleStart(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateTwoTestLoop(t, loop)

	// Second start attempt (from different goroutine) should fail with ErrLoopAlreadyRunning
	err = loop.Run(context.Background())
	if err != ErrLoopAlreadyRunning {
		t.Errorf("Expected ErrLoopAlreadyRunning, got %v", err)
	}

	harness.shutdown(t)
}

func TestDoubleShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateTwoTestLoop(t, loop)
	harness.shutdown(t)

	// Second shutdown should not error (idempotent via sync.Once)
	stopCtx, cancel := context.WithTimeout(context.Background(), alternateTwoTestTimeout)
	defer cancel()
	if err := loop.Shutdown(stopCtx); err != nil {
		t.Errorf("Second Shutdown() returned %v, want nil", err)
	}
}

func TestSubmitAfterShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateTwoTestLoop(t, loop)
	harness.shutdown(t)

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
	for range 100 {
		q.Push(func() {})
	}

	if q.Length() != 100 {
		t.Errorf("Expected length 100, got %d", q.Length())
	}

	// Pop all tasks
	for i := range 100 {
		task, ok := q.Pop()
		if !ok || task.Fn == nil {
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
	for i := range 100 {
		if !r.Push(func() {}) {
			t.Errorf("Push failed at iteration %d", i)
		}
	}

	if r.Length() != 100 {
		t.Errorf("Expected length 100, got %d", r.Length())
	}

	// Pop all
	for i := range 100 {
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
	for i := range 50 {
		c.tasks[i] = Task{Fn: func() {}}
		c.pos++
	}

	// Return with minimal clearing
	returnChunkFast(c)

	// Verify cleared slots
	for i := range 50 {
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

	harness := startAlternateTwoTestLoop(t, loop)

	// Run should not have returned yet
	select {
	case err := <-harness.runDone:
		harness.runDone <- err
		t.Fatalf("Run() returned early: %v", err)
	default:
		// OK - Run is still blocking
	}

	// Shutdown the loop
	harness.shutdown(t)
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

	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		t.Fatalf("Submit(start barrier) failed: %v", err)
	}
	waitAlternateTwoSignal(t, ready, "context-cancel loop start")

	// Cancel the context
	cancel()

	// Run should exit
	select {
	case err := <-runDone:
		if err != context.Canceled {
			t.Errorf("Run() returned %v, want context.Canceled", err)
		}
	case <-time.After(alternateTwoTestTimeout):
		t.Fatal("Run() did not exit after context cancel")
	}

	// State should be Terminating or Terminated after context cancel
	// Since Run() returns early on context cancel, state may not be Terminated yet
	state := loop.State()
	if state != StateTerminating {
		t.Errorf("Expected state Terminating, got %v", state)
	}
	loop.closeFDs()
}

func BenchmarkSubmit(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateTwoTestLoop(b, loop)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := loop.Submit(func() {}); err != nil {
			b.Fatalf("Submit() failed: %v", err)
		}
	}
	b.StopTimer()
	drained := make(chan struct{})
	if err := loop.Submit(func() { close(drained) }); err != nil {
		b.Fatalf("Submit(drain barrier) failed: %v", err)
	}
	waitAlternateTwoSignal(b, drained, "benchmark drain")
	harness.shutdown(b)
}

func BenchmarkLockFreeIngress_Push(b *testing.B) {
	q := NewLockFreeIngress()
	fn := func() {}

	// Retained exactly as the original longitudinal diagnostic. Queue length
	// grows with b.N, so this root is nonstationary and not comparison-valid.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(fn)
	}
}

func BenchmarkLockFreeIngress_PushBounded(b *testing.B) {
	const batchSize = 1024
	q := NewLockFreeIngress()
	fn := func() {}

	b.ReportAllocs()
	b.ResetTimer()
	for completed := 0; completed < b.N; {
		count := min(batchSize, b.N-completed)
		for range count {
			q.Push(fn)
		}
		b.StopTimer()
		for range count {
			if popped, ok := q.Pop(); !ok || popped.Fn == nil {
				b.Fatal("Pop() failed while bounding the ingress queue")
			}
		}
		if length := q.Length(); length != 0 {
			b.Fatalf("bounded ingress queue length = %d, want 0", length)
		}
		completed += count
		b.StartTimer()
	}
	b.StopTimer()
}

func BenchmarkLockFreeIngress_PushPop(b *testing.B) {
	q := NewLockFreeIngress()
	fn := func() {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(fn)
		if popped, ok := q.Pop(); !ok || popped.Fn == nil {
			b.Fatal("Pop() failed")
		}
	}
}

func BenchmarkMicrotaskRing_PushPop(b *testing.B) {
	r := NewMicrotaskRing()
	fn := func() {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !r.Push(fn) {
			b.Fatal("Push() failed")
		}
		if popped := r.Pop(); popped == nil {
			b.Fatal("Pop() failed")
		}
	}
}
