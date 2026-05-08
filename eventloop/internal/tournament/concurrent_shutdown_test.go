package tournament

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type terminalCallResult struct {
	err      error
	panicked bool
}

// TestConcurrentShutdown verifies that simultaneous terminal callers all
// return without panic or context expiry. Historical terminal errors are
// retained as measured dispositions rather than ignored.
func TestConcurrentShutdown(t *testing.T) {
	const callerCount = 10
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			start := time.Now()
			loop, cleanup := startTournamentTestLoop(t, impl)
			results := runConcurrentShutdownCalls(t, loop, callerCount)
			cleanup()

			passed := true
			errorCount := 0
			for _, result := range results {
				if result.panicked || errors.Is(result.err, context.Canceled) || errors.Is(result.err, context.DeadlineExceeded) {
					passed = false
				}
				if result.err != nil {
					errorCount++
					// ErrLoopTerminated is expected for concurrent callers that
					// arrive after the terminal completion is published.
					if strings.HasPrefix(impl.VariantID, "scheduler.main.") &&
						!strings.Contains(result.err.Error(), "loop has been terminated") {
						passed = false
					}
				}
			}
			GetResults().RecordTest(TestResult{
				TestName:       "ConcurrentShutdown",
				VariantID:      impl.VariantID,
				Implementation: impl.Name,
				Passed:         passed,
				Duration:       time.Since(start),
				Metrics: map[string]any{
					"callers":         callerCount,
					"terminal_errors": errorCount,
				},
			})
			if !passed {
				t.Error("a concurrent Shutdown caller violated the variant's terminal contract")
			}
		})
	}
}

func runConcurrentShutdownCalls(t *testing.T, loop EventLoop, callerCount int) []terminalCallResult {
	t.Helper()
	ready := make(chan struct{}, callerCount)
	start := make(chan struct{})
	results := make(chan terminalCallResult, callerCount)
	for range callerCount {
		go func() {
			ready <- struct{}{}
			<-start
			result := terminalCallResult{}
			completed := false
			defer func() {
				result.panicked = !completed
				results <- result
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result.err = loop.Shutdown(ctx)
			completed = true
		}()
	}
	waitTournamentCount(t, ready, callerCount, "concurrent Shutdown callers ready")
	close(start)
	collected := make([]terminalCallResult, 0, callerCount)
	deadline := time.NewTimer(35 * time.Second)
	defer deadline.Stop()
	for len(collected) < callerCount {
		select {
		case result := <-results:
			collected = append(collected, result)
		case <-deadline.C:
			t.Fatalf("concurrent Shutdown returned %d of %d results", len(collected), callerCount)
		}
	}
	return collected
}

// TestConcurrentShutdownWithSubmits records admission while terminal callers
// race with producers and proves conservation for graceful-drain variants.
func TestConcurrentShutdownWithSubmits(t *testing.T) {
	const stopperCount = 5
	const submitterCount = 5
	const submitsPerSubmitter = 1000
	const submissionCount = submitterCount * submitsPerSubmitter
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			startTime := time.Now()
			loop, cleanup := startTournamentTestLoop(t, impl)
			ready := make(chan struct{}, stopperCount+submitterCount)
			start := make(chan struct{})
			done := make(chan terminalCallResult, stopperCount+submitterCount)
			var accepted atomic.Int64
			var rejected atomic.Int64
			var executed atomic.Int64

			for range submitterCount {
				go func() {
					ready <- struct{}{}
					<-start
					result := terminalCallResult{}
					completed := false
					defer func() {
						result.panicked = !completed
						done <- result
					}()
					for range submitsPerSubmitter {
						if err := loop.Submit(func() { executed.Add(1) }); err != nil {
							rejected.Add(1)
						} else {
							accepted.Add(1)
						}
					}
					completed = true
				}()
			}
			for range stopperCount {
				go func() {
					ready <- struct{}{}
					<-start
					result := terminalCallResult{}
					completed := false
					defer func() {
						result.panicked = !completed
						done <- result
					}()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					result.err = loop.Shutdown(ctx)
					completed = true
				}()
			}

			waitTournamentCount(t, ready, stopperCount+submitterCount, "shutdown race callers ready")
			close(start)
			deadline := time.NewTimer(35 * time.Second)
			defer deadline.Stop()
			passedCalls := true
			for completed := range stopperCount + submitterCount {
				select {
				case result := <-done:
					if result.panicked || errors.Is(result.err, context.Canceled) || errors.Is(result.err, context.DeadlineExceeded) {
						passedCalls = false
					}
				case <-deadline.C:
					t.Fatalf("shutdown race completed %d of %d callers", completed, stopperCount+submitterCount)
				}
			}
			cleanup()

			acceptedCount := accepted.Load()
			rejectedCount := rejected.Load()
			executedCount := executed.Load()
			if acceptedCount+rejectedCount != submissionCount {
				t.Fatalf("submission accounting = %d accepted + %d rejected, want %d", acceptedCount, rejectedCount, submissionCount)
			}
			gracefulDrain := impl.HasCapability(CapabilityGracefulDrain)
			conserved := acceptedCount == executedCount
			status := TestStatusUnsupported
			passed := false
			if !passedCalls {
				status = TestStatusFailed
				t.Error("a shutdown-race caller panicked or exhausted its context")
			} else if gracefulDrain {
				passed = conserved
				status = TestStatusPassed
				if !passed {
					status = TestStatusFailed
					t.Errorf("shutdown race: accepted=%d executed=%d", acceptedCount, executedCount)
				}
			}
			GetResults().RecordTest(TestResult{
				TestName:       "ConcurrentShutdownWithSubmits",
				VariantID:      impl.VariantID,
				Implementation: impl.Name,
				Status:         status,
				Passed:         passed,
				Duration:       time.Since(startTime),
				Metrics: map[string]any{
					"accepted":               acceptedCount,
					"rejected":               rejectedCount,
					"executed":               executedCount,
					"graceful_drain_capable": gracefulDrain,
					"callers_ok":             passedCalls,
				},
			})
		})
	}
}

// TestConcurrentShutdownRepeated verifies construction and concurrent
// termination over repeated fresh instances while recording one contract cell.
func TestConcurrentShutdownRepeated(t *testing.T) {
	const iterations = 10
	const callerCount = 3
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			start := time.Now()
			passed := true
			for range iterations {
				loop, cleanup := startTournamentTestLoop(t, impl)
				for _, result := range runConcurrentShutdownCalls(t, loop, callerCount) {
					if result.panicked || errors.Is(result.err, context.Canceled) || errors.Is(result.err, context.DeadlineExceeded) {
						passed = false
					}
					if strings.HasPrefix(impl.VariantID, "scheduler.main.") && result.err != nil &&
						!strings.Contains(result.err.Error(), "loop has been terminated") {
						passed = false
					}
				}
				cleanup()
			}
			GetResults().RecordTest(TestResult{
				TestName:       "ConcurrentShutdownRepeated",
				VariantID:      impl.VariantID,
				Implementation: impl.Name,
				Passed:         passed,
				Duration:       time.Since(start),
				Metrics: map[string]any{
					"iterations": iterations,
					"callers":    callerCount,
				},
			})
			if !passed {
				t.Error("a repeated concurrent Shutdown caller violated the variant's terminal contract")
			}
		})
	}
}
