package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ===============================================
// EXPAND-020: process.nextTick() Tests
// ===============================================

func TestNextTick_Basic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	var called atomic.Bool
	done := make(chan struct{})

	go func() {
		if err := loop.Run(context.Background()); err != nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()

	err = js.NextTick(func() {
		called.Store(true)
		close(done)
	})
	if err != nil {
		t.Fatalf("NextTick failed: %v", err)
	}

	select {
	case <-done:
		if !called.Load() {
			t.Error("NextTick callback was not called")
		}
	case <-time.After(5 * time.Second):
		t.Error("NextTick callback did not execute in time")
	}

	loop.Shutdown(context.Background())
}

func TestNextTick_RunsBeforeMicrotask(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	var order []string
	var mu sync.Mutex
	done := make(chan struct{})
	var callCount atomic.Int32

	go func() {
		if err := loop.Run(context.Background()); err != nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()

	// Schedule a regular microtask first
	err = js.QueueMicrotask(func() {
		mu.Lock()
		order = append(order, "microtask")
		mu.Unlock()
		if callCount.Add(1) == 2 {
			close(done)
		}
	})
	if err != nil {
		t.Fatalf("QueueMicrotask failed: %v", err)
	}

	// Then schedule a nextTick - should run BEFORE microtask
	err = js.NextTick(func() {
		mu.Lock()
		order = append(order, "nextTick")
		mu.Unlock()
		if callCount.Add(1) == 2 {
			close(done)
		}
	})
	if err != nil {
		t.Fatalf("NextTick failed: %v", err)
	}

	select {
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		if len(order) != 2 {
			t.Fatalf("Expected 2 calls, got %d", len(order))
		}
		if order[0] != "nextTick" {
			t.Errorf("Expected nextTick to run first, got order: %v", order)
		}
		if order[1] != "microtask" {
			t.Errorf("Expected microtask to run second, got order: %v", order)
		}
	case <-time.After(5 * time.Second):
		t.Error("Callbacks did not execute in time")
	}

	loop.Shutdown(context.Background())
}

func TestNextTick_RunsBeforePromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	var order []string
	var mu sync.Mutex
	done := make(chan struct{})
	var callCount atomic.Int32

	go func() {
		if err := loop.Run(context.Background()); err != nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()

	// Create a resolved promise - its then handler is a microtask
	promise := js.Resolve("test")
	promise.Then(func(r Result) Result {
		mu.Lock()
		order = append(order, "promise.then")
		mu.Unlock()
		if callCount.Add(1) == 2 {
			close(done)
		}
		return nil
	}, nil)

	// Schedule a nextTick - should run BEFORE promise.then
	err = js.NextTick(func() {
		mu.Lock()
		order = append(order, "nextTick")
		mu.Unlock()
		if callCount.Add(1) == 2 {
			close(done)
		}
	})
	if err != nil {
		t.Fatalf("NextTick failed: %v", err)
	}

	select {
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		if len(order) != 2 {
			t.Fatalf("Expected 2 calls, got %d", len(order))
		}
		if order[0] != "nextTick" {
			t.Errorf("Expected nextTick to run first, got order: %v", order)
		}
	case <-time.After(5 * time.Second):
		t.Error("Callbacks did not execute in time")
	}

	loop.Shutdown(context.Background())
}

func TestNextTick_NilCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	// Nil callback should not error
	err = js.NextTick(nil)
	if err != nil {
		t.Fatalf("NextTick(nil) should not error, got: %v", err)
	}
}

func TestNextTick_MultipleCallbacksOrder(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	var order []int
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		if err := loop.Run(context.Background()); err != nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()

	// Schedule multiple nextTicks - should run in order
	for i := 1; i <= 5; i++ {
		i := i
		err = js.NextTick(func() {
			mu.Lock()
			order = append(order, i)
			if len(order) == 5 {
				close(done)
			}
			mu.Unlock()
		})
		if err != nil {
			t.Fatalf("NextTick %d failed: %v", i, err)
		}
	}

	select {
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		expected := []int{1, 2, 3, 4, 5}
		if len(order) != len(expected) {
			t.Fatalf("Expected %d calls, got %d", len(expected), len(order))
		}
		for i, v := range expected {
			if order[i] != v {
				t.Errorf("At index %d: expected %d, got %d", i, v, order[i])
			}
		}
	case <-time.After(5 * time.Second):
		t.Error("Callbacks did not execute in time")
	}

	loop.Shutdown(context.Background())
}

func TestNextTick_AfterLoopShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	go func() {
		_ = loop.Run(context.Background())
	}()

	loop.Shutdown(context.Background())
	time.Sleep(50 * time.Millisecond)

	err = js.NextTick(func() {
		t.Error("Should not execute after shutdown")
	})
	if err != ErrLoopTerminated {
		t.Errorf("Expected ErrLoopTerminated, got: %v", err)
	}
}

// ===============================================
// EXPAND-021: sleep/delay Promise Helper Tests
// ===============================================

func TestSleep_Basic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	var resolved atomic.Bool
	done := make(chan struct{})

	go func() {
		if err := loop.Run(context.Background()); err != nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()

	start := time.Now()
	promise := js.Sleep(50 * time.Millisecond)
	promise.Then(func(r Result) Result {
		resolved.Store(true)
		close(done)
		return nil
	}, nil)

	select {
	case <-done:
		elapsed := time.Since(start)
		if !resolved.Load() {
			t.Error("Sleep promise did not resolve")
		}
		if elapsed < 40*time.Millisecond {
			t.Errorf("Sleep resolved too quickly: %v", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Error("Sleep did not resolve in time")
	}

	loop.Shutdown(context.Background())
}

func TestSleep_ZeroDelay(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	var resolved atomic.Bool
	done := make(chan struct{})

	go func() {
		if err := loop.Run(context.Background()); err != nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()

	promise := js.Sleep(0)
	promise.Then(func(r Result) Result {
		resolved.Store(true)
		close(done)
		return nil
	}, nil)

	select {
	case <-done:
		if !resolved.Load() {
			t.Error("Sleep(0) promise did not resolve")
		}
	case <-time.After(5 * time.Second):
		t.Error("Sleep(0) did not resolve in time")
	}

	loop.Shutdown(context.Background())
}

func TestSleep_ResolvesWithNil(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	var result Result
	done := make(chan struct{})

	go func() {
		if err := loop.Run(context.Background()); err != nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()

	promise := js.Sleep(10 * time.Millisecond)
	promise.Then(func(r Result) Result {
		result = r
		close(done)
		return nil
	}, nil)

	select {
	case <-done:
		if result != nil {
			t.Errorf("Expected nil result, got: %v", result)
		}
	case <-time.After(5 * time.Second):
		t.Error("Sleep did not resolve in time")
	}

	loop.Shutdown(context.Background())
}

func TestSleep_Chaining(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	var callOrder []string
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		if err := loop.Run(context.Background()); err != nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()

	js.Sleep(10*time.Millisecond).
		Then(func(r Result) Result {
			mu.Lock()
			callOrder = append(callOrder, "first")
			mu.Unlock()
			return "value1"
		}, nil).
		Then(func(r Result) Result {
			mu.Lock()
			callOrder = append(callOrder, "second")
			mu.Unlock()
			close(done)
			return nil
		}, nil)

	select {
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		if len(callOrder) != 2 {
			t.Fatalf("Expected 2 calls, got %d", len(callOrder))
		}
		if callOrder[0] != "first" || callOrder[1] != "second" {
			t.Errorf("Unexpected order: %v", callOrder)
		}
	case <-time.After(5 * time.Second):
		t.Error("Chain did not complete in time")
	}

	loop.Shutdown(context.Background())
}

func TestSleep_MultipleConcurrent(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	var count atomic.Int32
	done := make(chan struct{})

	go func() {
		if err := loop.Run(context.Background()); err != nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()

	// Schedule multiple sleeps concurrently
	for i := 0; i < 5; i++ {
		js.Sleep(time.Duration(i*10)*time.Millisecond).Then(func(r Result) Result {
			if count.Add(1) == 5 {
				close(done)
			}
			return nil
		}, nil)
	}

	select {
	case <-done:
		if count.Load() != 5 {
			t.Errorf("Expected 5 resolutions, got %d", count.Load())
		}
	case <-time.After(5 * time.Second):
		t.Error("Not all sleeps resolved in time")
	}

	loop.Shutdown(context.Background())
}

// TestTimeout_Basic verifies that Timeout returns a promise that rejects after the delay.
func TestTimeout_Basic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	done := make(chan struct{})
	start := time.Now()
	var capturedErr error

	go func() {
		if err := loop.Run(context.Background()); err != nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()

	// Timeout should reject after delay
	js.Timeout(100*time.Millisecond).Then(
		func(r Result) Result {
			t.Error("Timeout should not resolve, it should reject")
			close(done)
			return nil
		},
		func(r Result) Result {
			capturedErr, _ = r.(error)
			close(done)
			return nil
		},
	)

	select {
	case <-done:
		elapsed := time.Since(start)
		if elapsed < 100*time.Millisecond {
			t.Errorf("Timeout resolved too early: %v", elapsed)
		}
		if capturedErr == nil {
			t.Error("Expected rejection with error")
		}
		var timeoutErr *TimeoutError
		if !errors.As(capturedErr, &timeoutErr) {
			t.Errorf("Expected TimeoutError, got: %T (%v)", capturedErr, capturedErr)
		}
	case <-time.After(5 * time.Second):
		t.Error("Timeout did not reject in time")
	}

	loop.Shutdown(context.Background())
}

// TestTimeout_WithRace verifies that Timeout works correctly with Race.
func TestTimeout_WithRace(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	done := make(chan struct{})
	var rejected atomic.Bool
	var resolved atomic.Bool

	go func() {
		if err := loop.Run(context.Background()); err != nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()

	// Race between a slow operation (200ms) and a timeout (50ms)
	// The timeout should win
	slowOp := js.Sleep(200*time.Millisecond).Then(func(r Result) Result {
		return "slow result"
	}, nil)
	timeout := js.Timeout(50 * time.Millisecond)

	js.Race([]*ChainedPromise{slowOp, timeout}).Then(
		func(r Result) Result {
			resolved.Store(true)
			close(done)
			return nil
		},
		func(r Result) Result {
			rejected.Store(true)
			close(done)
			return nil
		},
	)

	select {
	case <-done:
		if resolved.Load() {
			t.Error("Race should have rejected with timeout, not resolved with slow result")
		}
		if !rejected.Load() {
			t.Error("Race should have rejected with timeout")
		}
	case <-time.After(5 * time.Second):
		t.Error("Race did not complete in time")
	}

	loop.Shutdown(context.Background())
}

// TestTimeout_ZeroDelay verifies that Timeout with zero delay rejects immediately.
func TestTimeout_ZeroDelay(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	done := make(chan struct{})
	var rejected atomic.Bool

	go func() {
		if err := loop.Run(context.Background()); err != nil {
			t.Errorf("loop.Run failed: %v", err)
		}
	}()

	// Zero delay should still reject (not panic or hang)
	js.Timeout(0).Then(
		func(r Result) Result {
			t.Error("Timeout should reject, not resolve")
			close(done)
			return nil
		},
		func(r Result) Result {
			rejected.Store(true)
			close(done)
			return nil
		},
	)

	select {
	case <-done:
		if !rejected.Load() {
			t.Error("Zero-delay timeout should reject")
		}
	case <-time.After(2 * time.Second):
		t.Error("Zero-delay timeout did not reject in time")
	}

	loop.Shutdown(context.Background())
}
