package eventloop

import (
	"context"
	"testing"
	"time"
)

// BenchmarkAutoExit_ImmediateExit measures the time for Run() to return
// with no initial work when autoExit is enabled.
func BenchmarkAutoExit_ImmediateExit(b *testing.B) {
	for b.Loop() {
		loop, err := New(WithAutoExit(true))
		if err != nil {
			b.Fatalf("New: %v", err)
		}
		if err := loop.Run(context.Background()); err != nil {
			b.Fatalf("Run: %v", err)
		}
	}
}

// BenchmarkAutoExit_TimerFire measures the time for Run() to return
// after a timer fires.
func BenchmarkAutoExit_TimerFire(b *testing.B) {
	for b.Loop() {
		loop, err := New(WithAutoExit(true))
		if err != nil {
			b.Fatalf("New: %v", err)
		}
		_, err = loop.ScheduleTimer(0, func() {})
		if err != nil {
			b.Fatalf("ScheduleTimer: %v", err)
		}
		if err := loop.Run(context.Background()); err != nil {
			b.Fatalf("Run: %v", err)
		}
	}
}

// BenchmarkAutoExit_UnrefExit measures the time for Run() to return
// after unref'ing the only timer.
func BenchmarkAutoExit_UnrefExit(b *testing.B) {
	for b.Loop() {
		loop, err := New(WithAutoExit(true))
		if err != nil {
			b.Fatalf("New: %v", err)
		}
		timerID, err := loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			b.Fatalf("ScheduleTimer: %v", err)
		}
		done := make(chan error, 1)
		go func() { done <- loop.Run(context.Background()) }()
		// Wait for the timer to be registered.
		time.Sleep(time.Millisecond)
		if err := loop.UnrefTimer(timerID); err != nil {
			b.Fatalf("UnrefTimer: %v", err)
		}
		if err := <-done; err != nil {
			b.Fatalf("Run: %v", err)
		}
	}
}

// BenchmarkAutoExit_AliveCheckCost measures the overhead of the autoExit
// Alive() check in the hot path by comparing runFastPath iteration time
// with autoExit disabled (baseline) vs enabled.
func BenchmarkAutoExit_AliveCheckCost_Disabled(b *testing.B) {
	// Baseline: autoExit disabled, loop processes tasks.
	loop, err := New()
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)
	time.Sleep(time.Millisecond) // let loop start

	b.ResetTimer()
	for b.Loop() {
		loop.Submit(func() {})
	}
	b.StopTimer()
	cancel()
}

func BenchmarkAutoExit_AliveCheckCost_Enabled(b *testing.B) {
	// With autoExit enabled: loop checks Alive() each iteration.
	// Keep the loop alive with a long timer to prevent auto-exit.
	loop, err := New(WithAutoExit(true))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err = loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		b.Fatalf("ScheduleTimer: %v", err)
	}
	go loop.Run(ctx)
	time.Sleep(time.Millisecond) // let loop start

	b.ResetTimer()
	for b.Loop() {
		loop.Submit(func() {})
	}
	b.StopTimer()
	cancel()
}

// BenchmarkAutoExit_FastPathExit measures auto-exit in fast path mode.
func BenchmarkAutoExit_FastPathExit(b *testing.B) {
	for b.Loop() {
		loop, err := New(
			WithAutoExit(true),
			WithFastPathMode(FastPathForced),
		)
		if err != nil {
			b.Fatalf("New: %v", err)
		}
		_, err = loop.ScheduleTimer(0, func() {})
		if err != nil {
			b.Fatalf("ScheduleTimer: %v", err)
		}
		if err := loop.Run(context.Background()); err != nil {
			b.Fatalf("Run: %v", err)
		}
	}
}

// BenchmarkAutoExit_PollPathExit measures auto-exit in poll path mode.
func BenchmarkAutoExit_PollPathExit(b *testing.B) {
	for b.Loop() {
		loop, err := New(
			WithAutoExit(true),
			WithFastPathMode(FastPathDisabled),
		)
		if err != nil {
			b.Fatalf("New: %v", err)
		}
		_, err = loop.ScheduleTimer(0, func() {})
		if err != nil {
			b.Fatalf("ScheduleTimer: %v", err)
		}
		if err := loop.Run(context.Background()); err != nil {
			b.Fatalf("Run: %v", err)
		}
	}
}

// BenchmarkQuiescing_ScheduleTimer_NoAutoExit measures ScheduleTimer with autoExit
// disabled. The quiescing atomic.Bool is present in the struct but never set.
// This is the baseline for the quiescing check overhead measurement.
func BenchmarkQuiescing_ScheduleTimer_NoAutoExit(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)
	time.Sleep(time.Millisecond) // let loop start

	b.ResetTimer()
	for b.Loop() {
		id, scheduleErr := loop.ScheduleTimer(time.Hour, func() {})
		if scheduleErr != nil {
			b.Fatalf("ScheduleTimer: %v", scheduleErr)
		}
		_ = loop.CancelTimer(id)
	}
	b.StopTimer()
	cancel()
}

// BenchmarkQuiescing_ScheduleTimer_WithAutoExit measures ScheduleTimer with autoExit
// enabled. The quiescing flag is present and checked (atomic.Bool.Load) on every call
// but returns false (loop is kept alive). The delta vs NoAutoExit quantifies the
// overhead of the additional atomic load on the hot path.
func BenchmarkQuiescing_ScheduleTimer_WithAutoExit(b *testing.B) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Keep the loop alive so auto-exit doesn't trigger during the benchmark
	keepAlive, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		b.Fatalf("ScheduleTimer keepAlive: %v", err)
	}
	_ = keepAlive
	go loop.Run(ctx)
	time.Sleep(time.Millisecond) // let loop start

	b.ResetTimer()
	for b.Loop() {
		id, scheduleErr := loop.ScheduleTimer(time.Hour, func() {})
		if scheduleErr != nil {
			b.Fatalf("ScheduleTimer: %v", scheduleErr)
		}
		_ = loop.CancelTimer(id)
	}
	b.StopTimer()
	cancel()
}

// ============================================================================
// SECTION: Terminated-State Rejection Path Benchmarks
//
// Measures the cost of each API's rejection path when the loop has fully
// terminated (StateTerminated). After Run() returns with autoExit, the state
// is StateTerminated and quiescing is cleared (see terminateCleanup). Each
// API checks state first, so these benchmarks measure the state-check rejection
// path — the path callers hit when submitting work to a stopped loop.
// ============================================================================

// BenchmarkTerminated_RejectionPath_ScheduleTimer measures ScheduleTimer
// rejection on a terminated loop. The quiescing check misses (cleared by
// terminateCleanup), so the full path is exercised: timer pool allocation,
// then submitToQueue catches StateTerminated and returns ErrLoopTerminated.
func BenchmarkTerminated_RejectionPath_ScheduleTimer(b *testing.B) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	if err := loop.Run(context.Background()); err != nil {
		b.Fatalf("Run: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, scheduleErr := loop.ScheduleTimer(time.Hour, func() {})
		if scheduleErr == nil {
			b.Fatal("expected ErrLoopTerminated")
		}
	}
}

// BenchmarkTerminated_RejectionPath_RefTimer measures RefTimer rejection on a
// terminated loop. submitTimerRefChange checks state first (line 1632):
// StateTerminated -> ErrLoopTerminated. Fast: one atomic load + comparison.
func BenchmarkTerminated_RejectionPath_RefTimer(b *testing.B) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	if err := loop.Run(context.Background()); err != nil {
		b.Fatalf("Run: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		refErr := loop.RefTimer(0)
		if refErr == nil {
			b.Fatal("expected ErrLoopTerminated")
		}
	}
}

// BenchmarkTerminated_UnrefTimer_NotGated confirms UnrefTimer (ref=false)
// is rejected by the state check in submitTimerRefChange, NOT by the
// quiescing gate. Same fast path as RefTimer: atomic state load + comparison.
func BenchmarkTerminated_UnrefTimer_NotGated(b *testing.B) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	if err := loop.Run(context.Background()); err != nil {
		b.Fatalf("Run: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		unrefErr := loop.UnrefTimer(0)
		if unrefErr == nil {
			b.Fatal("expected ErrLoopTerminated")
		}
	}
}

// BenchmarkTerminated_RejectionPath_Promisify measures Promisify rejection on a
// terminated loop. Acquires promisifyMu, checks state (StateTerminated),
// creates a promise, rejects it, and returns. Heavier than RefTimer due to
// mutex acquisition and promise allocation.
func BenchmarkTerminated_RejectionPath_Promisify(b *testing.B) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	if err := loop.Run(context.Background()); err != nil {
		b.Fatalf("Run: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		p := loop.Promisify(context.Background(), func(ctx context.Context) (any, error) {
			return nil, nil
		})
		if p.State() != Rejected {
			b.Fatalf("expected rejected promise, got %v", p.State())
		}
	}
}

// BenchmarkTerminated_RejectionPath_submitToQueue measures SubmitInternal
// rejection on a terminated loop. submitToQueue acquires internalQueueMu,
// checks state (StateTerminated), releases, returns ErrLoopTerminated.
func BenchmarkTerminated_RejectionPath_submitToQueue(b *testing.B) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	if err := loop.Run(context.Background()); err != nil {
		b.Fatalf("Run: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		submitErr := loop.SubmitInternal(func() {})
		if submitErr == nil {
			b.Fatal("expected ErrLoopTerminated")
		}
	}
}
