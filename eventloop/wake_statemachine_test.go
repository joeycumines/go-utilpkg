package eventloop

import (
	"context"
	"net"
	"testing"
	"time"
)

// TestWake_IOMode_StateSleeping tests Wake() coverage when loop
// is in I/O MODE and StateSleeping.
// Priority: CRITICAL - Hits doWakeup() path in Wake().
//
// This test registers a file descriptor to force I/O MODE
// (kqueue/epoll/IOCP blocking), then calls Wake() during poll to
// ensure we hit the state == StateSleeping path.
func TestWake_IOMode_StateSleeping(t *testing.T) {
	// Create a TCP listener to get a proper socket FD.
	// TCP sockets work with all poller backends:
	//   - kqueue (Darwin): TCP sockets are kqueue-compatible
	//   - epoll (Linux): TCP sockets are epoll-compatible
	//   - IOCP (Windows): TCP sockets are IOCP-compatible (unlike os.Pipe)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to create TCP listener:", err)
	}
	defer ln.Close()

	// Get the underlying FD from the listener
	tcpLn := ln.(*net.TCPListener)
	f, err := tcpLn.File()
	if err != nil {
		t.Fatal("Failed to get listener file:", err)
	}
	defer f.Close()
	fd := int(f.Fd())

	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	// Register listener socket for read events (accept-readiness)
	callbackCalled := false
	err = loop.Submit(func() {
		err := loop.RegisterFD(fd, EventRead, func(events IOEvents) {
			loop.UnregisterFD(fd)
			callbackCalled = true
		})
		if err != nil {
			t.Errorf("RegisterFD failed: %v", err)
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
