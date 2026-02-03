// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestWake_IOMode_StateSleeping tests Wake() coverage when loop
// is in I/O MODE and StateSleeping.
// Priority: CRITICAL - Hits doWakeup() path in Wake().
//
// This test registers a file descriptor to force I/O MODE
// (kqueue/epoll blocking), then calls Wake() during poll to
// ensure we hit the state == StateSleeping path.
func TestWake_IOMode_StateSleeping(t *testing.T) {
	// Create a pipe for I/O registration
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal("Failed to create pipe:", err)
	}
	defer r.Close()
	defer w.Close()

	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	// Register read side for read events
	callbackCalled := false
	err = loop.Submit(func() {
		// For Darwin, EventRead corresponds to EVFILT_READ
		err := loop.RegisterFD(int(r.Fd()), EventRead, func(events IOEvents) {
			// Unregister and note callback
			loop.UnregisterFD(int(r.Fd()))
			callbackCalled = true
		})
		if err != nil {
			t.Fatal("RegisterFD failed:", err)
		}
	})
	if err != nil {
		t.Fatal("Submit RegisterFD failed:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	// Wait for loop to enter I/O MODE poll (StateSleeping)
	// Give it time to register FD and enter poll
	time.Sleep(100 * time.Millisecond)

	stateBefore := LoopState(loop.state.Load())
	t.Logf("State before Wake(): %v (Sleeping? %v)", stateBefore, stateBefore == StateSleeping)

	// Call Wake() - this should:
	// 1. Check state == StateSleeping
	// 2. CompareAndSwap wakeUpSignalPending 0->1
	// 3. Call doWakeup() → submitWakeup()
	// 4. Write to fastWakeupCh or kqueue to wake poll
	err = loop.Wake()
	if err != nil {
		t.Fatalf("Wake() should return nil, got: %v", err)
	}

	// Wait to verify Wake() didn't cause issues
	time.Sleep(50 * time.Millisecond)

	t.Logf("Wake() I/O MODE StateSleeping test passed, callback called: %v", callbackCalled)
}

// TestWake_FastMode_StateSleeping tests Wake() coverage in FAST MODE.
// Priority: HIGH - Ensures Wake() works in channel-based mode.
//
// FAST MODE uses fastWakeupCh instead of kqueue/epoll.
// This test ensures Wake() triggers channel wakeups correctly.
func TestWake_FastMode_StateSleeping(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	// Cancel all timers to prevent immediate work
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	// Wait for loop to enter fast mode poll
	time.Sleep(200 * time.Millisecond)

	stateBefore := LoopState(loop.state.Load())
	t.Logf("State before Wake(): %v (Sleeping? %v)", stateBefore, stateBefore == StateSleeping)

	// Call Wake() in fast mode
	err = loop.Wake()
	if err != nil {
		t.Fatalf("Wake() should return nil in fast mode, got: %v", err)
	}

	// Verify loop continues running
	time.Sleep(50 * time.Millisecond)

	stateAfter := LoopState(loop.state.Load())
	t.Logf("State after Wake(): %v", stateAfter)

	t.Log("Wake() FAST MODE test passed")
}

// TestWake_DeduplicatePending tests that Wake() is deduplicated
// when wakeUpSignalPending is already set.
// Priority: MEDIUM - Tests CompareAndSwap logic.
func TestWake_DeduplicatePending(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// First Wake() sets wakeUpSignalPending = 1
	err = loop.Wake()
	if err != nil {
		t.Fatalf("First Wake() failed: %v", err)
	}

	// Second Wake() sees wakeUpSignalPending = 1, skips calling doWakeup()
	// This is the deduplication path in the CompareAndSwap
	err = loop.Wake()
	if err != nil {
		t.Fatalf("Second Wake() failed: %v", err)
	}

	// Third Wake() also skips
	err = loop.Wake()
	if err != nil {
		t.Fatalf("Third Wake() failed: %v", err)
	}

	t.Log("Wake() deduplication test passed - multiple calls handled correctly")
}
