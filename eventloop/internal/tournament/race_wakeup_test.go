//go:build linux || darwin

package tournament

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRaceWakeup verifies that tasks submitted exactly as the loop enters
// the sleep transition are not lost (no lost wakeup).
//
// This is T2: Correctness - Check-Then-Sleep Race Test
func TestRaceWakeup(t *testing.T) {
	for _, impl := range Implementations() {
		impl := impl // capture
		t.Run(impl.Name, func(t *testing.T) {
			t.Parallel()
			testRaceWakeup(t, impl)
		})
	}
}

func testRaceWakeup(t *testing.T, impl Implementation) {
	const iterations = 100

	start := time.Now()
	failures := 0

	for i := 0; i < iterations; i++ {
		if !runSingleWakeupTest(t, impl) {
			failures++
		}
	}

	passed := failures == 0
	result := TestResult{
		TestName:       "RaceWakeup",
		Implementation: impl.Name,
		Passed:         passed,
		Duration:       time.Since(start),
		Metrics: map[string]interface{}{
			"iterations": iterations,
			"failures":   failures,
		},
	}
	if !passed {
		result.Error = "lost wakeup detected"
	}
	GetResults().RecordTest(result)

	if failures > 0 {
		t.Errorf("%s: Lost wakeup in %d/%d iterations", impl.Name, failures, iterations)
	}
}

func runSingleWakeupTest(t *testing.T, impl Implementation) bool {
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

	// Wait for loop to be running and potentially sleeping
	time.Sleep(1 * time.Millisecond)

	var executed atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Submit a task that should wake the loop
	err = loop.Submit(func() {
		executed.Store(true)
		wg.Done()
	})
	if err != nil {
		// Rejected is also acceptable if loop is shutting down
		wg.Done()
		_ = loop.Shutdown(ctx)
		runWg.Wait()
		return true
	}

	// Wait for execution with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Task executed
	case <-time.After(500 * time.Millisecond):
		// Potential lost wakeup
		_ = loop.Shutdown(ctx)
		runWg.Wait()
		return false
	}

	stopCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	return executed.Load()
}

// TestRaceWakeup_Aggressive tests the wakeup race under aggressive timing conditions.
func TestRaceWakeup_Aggressive(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping aggressive test in short mode")
	}

	for _, impl := range Implementations() {
		impl := impl
		t.Run(impl.Name, func(t *testing.T) {
			testRaceWakeupAggressive(t, impl)
		})
	}
}

func testRaceWakeupAggressive(t *testing.T, impl Implementation) {
	const iterations = 500
	const concurrentSubmitters = 10

	start := time.Now()
	var failures atomic.Int64

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

	for i := 0; i < concurrentSubmitters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations/concurrentSubmitters; j++ {
				executed := make(chan struct{}, 1)

				err := loop.Submit(func() {
					select {
					case executed <- struct{}{}:
					default:
					}
				})
				if err != nil {
					continue // Rejected is ok
				}

				select {
				case <-executed:
					// Good - task was executed
				case <-time.After(100 * time.Millisecond):
					failures.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	failCount := failures.Load()
	passed := failCount == 0

	result := TestResult{
		TestName:       "RaceWakeup_Aggressive",
		Implementation: impl.Name,
		Passed:         passed,
		Duration:       time.Since(start),
		Metrics: map[string]interface{}{
			"iterations":         iterations,
			"concurrent_submits": concurrentSubmitters,
			"failures":           failCount,
		},
	}
	if !passed {
		result.Error = "lost wakeup under aggressive conditions"
	}
	GetResults().RecordTest(result)

	if failCount > 0 {
		t.Errorf("%s: Lost wakeup in %d tasks", impl.Name, failCount)
	}
}
