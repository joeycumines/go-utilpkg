package tournament

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestShutdownConservation verifies that all submitted tasks are either
// executed or explicitly rejected during shutdown. Zero data loss allowed.
//
// This is T1: Correctness - Shutdown Conservation Test
func TestShutdownConservation(t *testing.T) {
	for _, impl := range Implementations() {
		impl := impl // capture
		t.Run(impl.Name, func(t *testing.T) {
			t.Parallel()
			testShutdownConservation(t, impl)
		})
	}
}

func testShutdownConservation(t *testing.T, impl Implementation) {
	const N = 10000 // Number of tasks to submit
	const numProducers = 4

	start := time.Now()

	loop, err := impl.Factory()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	if err := loop.Start(ctx); err != nil {
		t.Fatalf("Failed to start loop: %v", err)
	}

	var executed atomic.Int64
	var rejected atomic.Int64
	var submitted atomic.Int64
	var wg sync.WaitGroup

	// Start producers
	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < N/numProducers; i++ {
				err := loop.Submit(func() {
					executed.Add(1)
				})
				if err != nil {
					rejected.Add(1)
				} else {
					submitted.Add(1)
				}
			}
		}()
	}

	// Let some tasks execute
	time.Sleep(10 * time.Millisecond)

	// Initiate shutdown
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	stopErr := loop.Stop(stopCtx)
	wg.Wait()

	if stopErr != nil && stopErr != context.DeadlineExceeded {
		t.Logf("Stop returned: %v", stopErr)
	}

	// Verify conservation: submitted = executed + rejected is NOT the invariant
	// The invariant is: every submitted task was either executed OR rejected
	// Tasks submitted successfully must be executed
	exec := executed.Load()
	rej := rejected.Load()
	sub := submitted.Load()

	// The key invariant: submitted == executed (for successful submissions)
	// Rejected tasks were never accepted, so they don't count
	if exec != sub {
		t.Errorf("%s: Conservation violated! Submitted: %d, Executed: %d, Lost: %d",
			impl.Name, sub, exec, sub-exec)
	}

	result := TestResult{
		TestName:       "ShutdownConservation",
		Implementation: impl.Name,
		Passed:         exec == sub,
		Duration:       time.Since(start),
		Metrics: map[string]interface{}{
			"submitted": sub,
			"executed":  exec,
			"rejected":  rej,
		},
	}
	if exec != sub {
		result.Error = "task conservation violated"
	}
	GetResults().RecordTest(result)

	t.Logf("%s: Submitted=%d, Executed=%d, Rejected=%d", impl.Name, sub, exec, rej)
}

// TestShutdownConservation_Stress runs the conservation test under heavy load.
// NOTE: AlternateTwo may fail this test - it trades off some correctness for performance.
func TestShutdownConservation_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	for _, impl := range Implementations() {
		impl := impl
		t.Run(impl.Name, func(t *testing.T) {
			for i := 0; i < 10; i++ {
				t.Run("Iteration", func(t *testing.T) {
					testShutdownConservation(t, impl)
				})
			}
		})
	}
}
