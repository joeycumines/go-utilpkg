package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

// TestDrainMicrotasksSafetyThresholdDiagnostic verifies the exhaustive-draining
// liveness property: drainMicrotasks drains without any budget cap (matching
// JavaScript's ability to starve the event loop with recursive microtasks), and
// its 100000-callback safety counter emits one error diagnostic WITHOUT
// stopping or yielding.
//
// The test schedules a self-rescheduling microtask that self-limits at
// safetyThreshold+100 callbacks (so the test provably cannot hang, even if the
// safety logic were broken), then asserts:
//   - draining continued PAST the threshold (count reached the limit, proving no
//     artificial stop at 100000);
//   - the loop did not hang (done fired);
//   - the safety-threshold diagnostic was emitted EXACTLY ONCE.
func TestDrainMicrotasksSafetyThresholdDiagnostic(t *testing.T) {
	const want = "eventloop: microtask drain exceeded safety threshold, possible infinite loop in callback"
	diagnostics := make(chan *testEvent, 2)
	logger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(event *testEvent) error {
			if event.message == want {
				diagnostics <- event
			}
			return nil
		})),
	).Logger()
	loop, err := New(WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runCh := make(chan error, 1)
	go func() { runCh <- loop.Run(ctx) }()
	waitLoopOwnerTurnT(t, loop)

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

	event := waitContractValue(t, diagnostics, "safety-threshold diagnostic")
	if event.level != logiface.LevelError {
		t.Fatalf("diagnostic level = %v, want %v", event.level, logiface.LevelError)
	}
	if got := event.fields["component"]; got != "eventloop" {
		t.Fatalf("diagnostic component = %#v, want eventloop", got)
	}
	select {
	case extra := <-diagnostics:
		t.Fatalf("unexpected duplicate safety-threshold diagnostic: %#v", extra)
	default:
	}

	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	cancel()
	if err := waitContractValue(t, runCh, "safety-threshold loop completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// TestDrain_ComplexInterleaving verifies the alternating-batch draining model
// handles a complex interleaving: a microtask that schedules BOTH another
// microtask AND a nextTick. The new microtask should run in the same microtask
// batch (inner loop is unbounded), while the nextTick should NOT preempt it —
// it runs in the next nextTick batch.
//
// Expected order: microtask-1, microtask-2, nextTick-1
//
// This matches Node.js v26.5.0 behavior. The test validates two
// invariants: (1) microtask-2 runs in the same microtask batch as microtask-1
// (inner loop is unbounded), and (2) nextTick-1 is deferred to the next
// nextTick batch rather than preempting microtask-2.
func TestDrain_ComplexInterleaving(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var order []string
	var mu sync.Mutex
	completed := make(chan error, 1)
	var completeOnce sync.Once
	complete := func(err error) { completeOnce.Do(func() { completed <- err }) }

	appendOrder := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitLoopOwnerTurnT(t, loop)

	// microtask-1: schedules both another microtask (microtask-2) and a nextTick.
	if err := loop.Submit(func() {
		if err := js.QueueMicrotask(func() {
			appendOrder("microtask-1")

			// Schedule another microtask — should run in the SAME microtask batch
			// (inner loop is unbounded, drains all microtasks before checking nextTick).
			if err := js.QueueMicrotask(func() {
				appendOrder("microtask-2")
			}); err != nil {
				complete(err)
				return
			}

			// Schedule a nextTick — should NOT preempt microtask-2.
			// It runs in the NEXT nextTick batch.
			if err := loop.ScheduleNextTick(func() {
				appendOrder("nextTick-1")
				complete(nil)
			}); err != nil {
				complete(err)
			}
		}); err != nil {
			complete(err)
		}
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if err := waitContractValue(t, completed, "complex microtask interleaving"); err != nil {
		t.Fatalf("nested scheduling: %v", err)
	}
	if err := loop.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "complex interleaving Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()

	expected := []string{"microtask-1", "microtask-2", "nextTick-1"}
	if len(got) != len(expected) {
		t.Fatalf("Expected %d events, got %d: %v", len(expected), len(got), got)
	}
	for i, ev := range expected {
		if got[i] != ev {
			t.Errorf("order[%d]: expected %q, got %q", i, ev, got[i])
		}
	}
}
