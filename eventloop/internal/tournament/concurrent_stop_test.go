//go:build linux || darwin

package tournament

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestConcurrentStop verifies that 10 goroutines calling Stop() simultaneously
// all return without hanging or panicking.
// This is T7: Robustness - Concurrent Stop Test
func TestConcurrentStop(t *testing.T) {
	for _, impl := range Implementations() {
		impl := impl
		t.Run(impl.Name, func(t *testing.T) {
			t.Parallel()
			testConcurrentStop(t, impl)
		})
	}
}

func testConcurrentStop(t *testing.T, impl Implementation) {
	const numStoppers = 10
	const timeout = 10 * time.Second

	start := time.Now()

	loop, err := impl.Factory()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	// Submit some tasks to keep loop busy
	for i := 0; i < 100; i++ {
		_ = loop.Submit(func() {
			time.Sleep(1 * time.Millisecond)
		})
	}

	var wg sync.WaitGroup
	var panicked atomic.Bool
	var completed atomic.Int64
	var errors atomic.Int64

	// Launch concurrent stoppers
	for i := 0; i < numStoppers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicked.Store(true)
					t.Logf("Stop() panicked: %v", r)
				}
			}()

			stopCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			err := loop.Shutdown(stopCtx)
			if err != nil && err != context.DeadlineExceeded {
				// ErrLoopTerminated is expected for subsequent callers
				errors.Add(1)
			}
			completed.Add(1)
		}()
	}

	// Wait for all stoppers with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All stoppers completed
	case <-time.After(timeout + 5*time.Second):
		t.Fatal("Concurrent Stop() calls hung")
	}

	comp := completed.Load()
	passed := comp == numStoppers && !panicked.Load()

	result := TestResult{
		TestName:       "ConcurrentStop",
		Implementation: impl.Name,
		Passed:         passed,
		Duration:       time.Since(start),
		Metrics: map[string]interface{}{
			"num_stoppers": numStoppers,
			"completed":    comp,
			"panicked":     panicked.Load(),
			"errors":       errors.Load(),
		},
	}
	if !passed {
		result.Error = "concurrent stop failed"
	}
	GetResults().RecordTest(result)

	if !passed {
		t.Errorf("%s: Concurrent stop failed - completed=%d, panicked=%v",
			impl.Name, comp, panicked.Load())
	}
}

// TestConcurrentStop_WithSubmits tests Stop() racing with Submit().
func TestConcurrentStop_WithSubmits(t *testing.T) {
	for _, impl := range Implementations() {
		impl := impl
		t.Run(impl.Name, func(t *testing.T) {
			testConcurrentStopWithSubmits(t, impl)
		})
	}
}

func testConcurrentStopWithSubmits(t *testing.T, impl Implementation) {
	const numStoppers = 5
	const numSubmitters = 5
	const submitsPerGoroutine = 1000
	const timeout = 10 * time.Second

	start := time.Now()

	loop, err := impl.Factory()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	var wg sync.WaitGroup
	var stopCompleted atomic.Int64
	var submitCompleted atomic.Int64
	var panicked atomic.Bool

	// Launch submitters
	for i := 0; i < numSubmitters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicked.Store(true)
				}
			}()

			for j := 0; j < submitsPerGoroutine; j++ {
				_ = loop.Submit(func() {})
				submitCompleted.Add(1)
			}
		}()
	}

	// Brief delay then launch stoppers
	time.Sleep(1 * time.Millisecond)

	for i := 0; i < numStoppers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicked.Store(true)
				}
			}()

			stopCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			_ = loop.Shutdown(stopCtx)
			stopCompleted.Add(1)
		}()
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout + 5*time.Second):
		t.Fatal("Test hung")
	}
	runWg.Wait()

	passed := stopCompleted.Load() == numStoppers && !panicked.Load()

	result := TestResult{
		TestName:       "ConcurrentStop_WithSubmits",
		Implementation: impl.Name,
		Passed:         passed,
		Duration:       time.Since(start),
		Metrics: map[string]interface{}{
			"num_stoppers":   numStoppers,
			"num_submitters": numSubmitters,
			"stop_completed": stopCompleted.Load(),
			"submits":        submitCompleted.Load(),
			"panicked":       panicked.Load(),
		},
	}
	if !passed {
		result.Error = "concurrent stop with submits failed"
	}
	GetResults().RecordTest(result)

	if !passed {
		t.Errorf("%s: Failed - stopCompleted=%d, panicked=%v",
			impl.Name, stopCompleted.Load(), panicked.Load())
	}
}

// TestConcurrentStop_Repeated tests multiple start/stop cycles.
func TestConcurrentStop_Repeated(t *testing.T) {
	for _, impl := range Implementations() {
		impl := impl
		t.Run(impl.Name, func(t *testing.T) {
			testConcurrentStopRepeated(t, impl)
		})
	}
}

func testConcurrentStopRepeated(t *testing.T, impl Implementation) {
	const iterations = 10

	start := time.Now()
	var panicked atomic.Bool

	for i := 0; i < iterations; i++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					panicked.Store(true)
					t.Logf("Iteration %d panicked: %v", i, r)
				}
			}()

			loop, err := impl.Factory()
			if err != nil {
				t.Fatalf("Failed to create loop: %v", err)
			}

			ctx := context.Background()
			var runWg sync.WaitGroup
			runWg.Add(1)
			go func() {
				loop.Run(ctx)
				runWg.Done()
			}()

			// Brief work
			_ = loop.Submit(func() {})

			// Concurrent stops
			var wg sync.WaitGroup
			for j := 0; j < 3; j++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					defer cancel()
					_ = loop.Shutdown(stopCtx)
				}()
			}
			wg.Wait()
		}()
	}

	passed := !panicked.Load()

	result := TestResult{
		TestName:       "ConcurrentStop_Repeated",
		Implementation: impl.Name,
		Passed:         passed,
		Duration:       time.Since(start),
		Metrics: map[string]interface{}{
			"iterations": iterations,
			"panicked":   panicked.Load(),
		},
	}
	if !passed {
		result.Error = "repeated start/stop cycles failed"
	}
	GetResults().RecordTest(result)

	if !passed {
		t.Errorf("%s: Repeated cycles failed", impl.Name)
	}
}
