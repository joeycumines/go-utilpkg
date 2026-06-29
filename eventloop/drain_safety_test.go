package eventloop

import (
	"bytes"
	"context"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestDrainMicrotasks_SafetyThresholdWarning verifies the exhaustive-draining
// liveness property: drainMicrotasks drains without any budget cap (matching
// JavaScript's ability to starve the event loop with recursive microtasks), and
// its 100000-callback safety counter logs a warning exactly once WITHOUT
// stopping or yielding.
//
// The test schedules a self-rescheduling microtask that self-limits at
// safetyThreshold+100 callbacks (so the test provably cannot hang, even if the
// safety logic were broken), then asserts:
//   - draining continued PAST the threshold (count reached the limit, proving no
//     artificial stop at 100000);
//   - the loop did not hang (done fired);
//   - the safety-threshold warning was logged EXACTLY ONCE.
//
// The warning is observed via the stdlib log fallback: with no logger attached,
// Loop.logError falls back to log.Printf, whose output is captured here through a
// scoped log.SetOutput swap (restored via defer). The loop is fully stopped
// (Shutdown + <-runCh) before the captured buffer is read, so no further log
// writes race the assertion.
func TestDrainMicrotasks_SafetyThresholdWarning(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// No WithLogger => logError uses the log.Printf fallback, captured below.

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Capture the stdlib log output (logError's fallback path).
	var logBuf bytes.Buffer
	origOut := log.Writer()
	origFlags := log.Flags()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(origOut)
		log.SetFlags(origFlags)
	}()

	runCh := make(chan error, 1)
	go func() { runCh <- loop.Run(ctx) }()
	waitForRunning(t, loop)

	const safetyThreshold = 100000 // matches drainMicrotasks' safetyThreshold
	const limit = safetyThreshold + 100
	var count atomic.Int64
	done := make(chan struct{})
	var once sync.Once

	// Self-rescheduling microtask. It self-limits at `limit` executions so the
	// test cannot hang regardless of the safety logic.
	var fn func()
	fn = func() {
		if count.Add(1) >= int64(limit) {
			once.Do(func() { close(done) })
			return
		}
		if err := js.QueueMicrotask(fn); err != nil {
			once.Do(func() { close(done) }) // loop terminated unexpectedly
			return
		}
	}
	if err := js.QueueMicrotask(fn); err != nil {
		t.Fatalf("initial QueueMicrotask failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatalf("drain stalled/hung: count=%d/%d", count.Load(), limit)
	}

	// Prove draining continued PAST the safety threshold (no artificial stop).
	if got := count.Load(); got != int64(limit) {
		t.Fatalf("draining did not continue past threshold: executed %d, expected %d", got, limit)
	}

	// Stop the loop before reading the captured log so no logError write races.
	if err := loop.Shutdown(context.Background()); err != nil && err != ErrLoopTerminated {
		t.Logf("Shutdown: %v", err)
	}
	cancel()
	<-runCh

	out := logBuf.String()
	const want = "microtask drain exceeded safety threshold"
	if !strings.Contains(out, want) {
		t.Errorf("safety-threshold warning was not logged. captured log:\n%s", out)
	}
	// The warning must fire exactly once (drainMicrotasks uses a one-shot guard).
	if c := strings.Count(out, want); c != 1 {
		t.Errorf("safety-threshold warning logged %d times, want exactly 1. captured log:\n%s", c, out)
	}
}
