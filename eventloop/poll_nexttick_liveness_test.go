package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestPoll_NextTickQueueLivenessCheck verifies that poll() does NOT block when
// only the nextTickQueue has pending work. This was a liveness bugfix (commit
// ec40e5f) — without the !l.nextTickQueue.IsEmpty() check in poll() line 1231,
// the loop could block until the next timer or external wakeup, delaying
// nextTick execution.
//
// The test uses the PrePollSleep test hook to deterministically schedule a
// nextTick callback right before poll() checks queue emptiness. If the check
// is present, poll() returns immediately and the nextTick runs promptly.
// If the check is removed, poll() blocks and the nextTick is delayed.
//
// Mutation verification: removing !l.nextTickQueue.IsEmpty() from poll()
// line 1231 causes this test to FAIL (nextTick delayed beyond threshold).
func TestPoll_NextTickQueueLivenessCheck(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var nextTickTime time.Time
	var mu sync.Mutex
	done := make(chan struct{})
	var once sync.Once

	// PrePollSleep is called right before poll() checks queue emptiness.
	// We schedule a nextTick here so that the check at line 1231 should
	// see !l.nextTickQueue.IsEmpty() == true and return immediately.
	// The hook fires on every poll() entry, but we only schedule once
	// to avoid recovered panics from duplicate close(done).
	pollEntered := make(chan struct{}, 1)
	var scheduleOnce sync.Once
	loop.testHooks = &loopTestHooks{
		PrePollSleep: func() {
			// Schedule a nextTick exactly once. This must happen BEFORE poll's
			// queue check so the check at line 1231 sees pending nextTick work.
			scheduleOnce.Do(func() {
				_ = loop.ScheduleNextTick(func() {
					mu.Lock()
					nextTickTime = time.Now()
					mu.Unlock()
					once.Do(func() { close(done) })
				})
			})
			// Signal that PrePollSleep was called (every time, not just once).
			select {
			case pollEntered <- struct{}{}:
			default:
			}
		},
	}

	var pollStartTime time.Time
	go func() {
		_ = loop.Run(ctx)
	}()

	// Wait for the loop to start and enter poll() at least once.
	// The PrePollSleep hook fires on every poll() entry.
	select {
	case <-pollEntered:
		pollStartTime = time.Now()
	case <-time.After(5 * time.Second):
		t.Fatal("PrePollSleep was never called — loop did not enter poll()")
	}

	defer loop.Shutdown(context.Background())

	// If the nextTickQueue.IsEmpty() check works, poll() returns immediately
	// and the nextTick runs within a short time.
	// If the check is removed, poll() blocks until the poll timeout (which
	// could be seconds with no timers), and the nextTick is delayed.
	select {
	case <-done:
		mu.Lock()
		elapsed := nextTickTime.Sub(pollStartTime)
		mu.Unlock()
		// The nextTick should run within 100ms of poll entry.
		// If the check is missing, poll blocks for much longer (seconds).
		if elapsed > 100*time.Millisecond {
			t.Errorf("nextTick delayed %v after poll entry — nextTickQueue check may be missing from poll()", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("nextTick never executed — poll() blocked despite pending nextTickQueue work")
	}
}
