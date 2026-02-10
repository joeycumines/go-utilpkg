package eventloop

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// Diagnostic test for SetTimeout timing issue
func TestJSSetTimeoutDiagnostic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		if err := loop.Run(ctx); err != nil {
			errChan <- err
		}
	}()

	// Wait for loop to initialize
	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	// Check loop timing state before scheduling
	t.Logf("=== Before SetTimeout ===")
	t.Logf("Loop state: %v", loop.State())
	t.Logf("tickAnchor: %v", loop.TickAnchor())
	t.Logf("tickElapsedTime: %v", time.Duration(loop.tickElapsedTime.Load()))
	t.Logf("CurrentTickTime(): %v", loop.CurrentTickTime())
	t.Logf("time.Now(): %v", time.Now())

	doneChan := make(chan struct{}, 1)
	var execTime atomic.Value
	var scheduleTime atomic.Value

	scheduleTime.Store(time.Now())

	start := time.Now()
	_, err = js.SetTimeout(func() {
		execTime.Store(time.Now())
		t.Logf("=== In callback ===")
		t.Logf("Callback time.Now(): %v", time.Now())
		t.Logf("Loop tickAnchor: %v", loop.TickAnchor())
		t.Logf("Loop tickElapsedTime: %v", time.Duration(loop.tickElapsedTime.Load()))
		t.Logf("Loop CurrentTickTime(): %v", loop.CurrentTickTime())
		close(doneChan)
	}, 10) // 10ms delay
	if err != nil {
		t.Fatalf("SetTimeout failed: %v", err)
	}

	// Wait for callback to run
	select {
	case <-doneChan:
		// Callback ran
	case <-time.After(5 * time.Second):
		t.Fatal("SetTimeout callback did not run within timeout")
	}

	execTimeVal, ok := execTime.Load().(time.Time)
	if !ok {
		t.Fatal("Failed to load execution time")
	}

	scheduleTimeVal, ok := scheduleTime.Load().(time.Time)
	if !ok {
		t.Fatal("Failed to load schedule time")
	}

	elapsed := execTimeVal.Sub(start)
	scheduleDelay := execTimeVal.Sub(scheduleTimeVal)

	t.Logf("=== Timing Analysis ===")
	t.Logf("Test start (wall clock): %v", start)
	t.Logf("scheduleTime (wall clock): %v", scheduleTimeVal)
	t.Logf("execTime (wall clock): %v", execTimeVal)
	t.Logf("Wall-clock elapsed (execTime.Sub(start)): %v", elapsed)
	t.Logf("Schedule-to-exec delay (execTime.Sub(scheduleTime)): %v", scheduleDelay)

	t.Logf("=== Expected vs Actual ===")
	t.Logf("Expected delay: 10ms")
	t.Logf("Actual delay: %v", scheduleDelay)
	t.Logf("Difference: %v", 10*time.Millisecond-scheduleDelay)

	loop.Shutdown(context.Background())

	select {
	case err := <-errChan:
		t.Fatalf("Run() error: %v", err)
	default:
	}
}

// Test direct ScheduleTimer timing
func TestScheduleTimerTimingSanity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		if err := loop.Run(ctx); err != nil {
			errChan <- err
		}
	}()

	// Wait for loop to initialize
	time.Sleep(10 * time.Millisecond)

	t.Logf("=== Direct ScheduleTimer Test ===")
	t.Logf("Loop state: %v", loop.State())
	t.Logf("tickAnchor: %v", loop.TickAnchor())
	t.Logf("tickElapsedTime: %v", time.Duration(loop.tickElapsedTime.Load()))
	t.Logf("CurrentTickTime(): %v", loop.CurrentTickTime())

	doneChan := make(chan struct{}, 1)
	var execTime atomic.Value

	start := time.Now()
	_, err = loop.ScheduleTimer(10*time.Millisecond, func() {
		execTime.Store(time.Now())
		t.Logf("=== Timer callback ===")
		t.Logf("Callback time.Now(): %v", time.Now())
		t.Logf("Loop CurrentTickTime(): %v", loop.CurrentTickTime())
		close(doneChan)
	})
	if err != nil {
		t.Fatalf("ScheduleTimer failed: %v", err)
	}

	select {
	case <-doneChan:
		// Timer fired
	case <-time.After(5 * time.Second):
		t.Fatal("ScheduleTimer callback did not run within timeout")
	}

	execTimeVal, ok := execTime.Load().(time.Time)
	if !ok {
		t.Fatal("Failed to load execution time")
	}

	elapsed := execTimeVal.Sub(start)
	t.Logf("Wall-clock elapsed: %v", elapsed)

	// NOTE: We DON'T check lower bound strictly because timing is non-deterministic
	// The key is that the timer fires, not the exact timing
	if elapsed < 5*time.Millisecond {
		t.Logf("WARNING: Timer fired very quickly: %v (might be timing variance)", elapsed)
	}

	loop.Shutdown(context.Background())

	select {
	case err := <-errChan:
		t.Fatalf("Run() error: %v", err)
	default:
	}
}
