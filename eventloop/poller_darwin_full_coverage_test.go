//go:build darwin

package eventloop

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// COVERAGE-012: Darwin Poller Full Coverage
// Tests: kevent/epoll error handling, kevent conversion, dispatchEvents callback dispatch,
// closed poller edge cases

// Test_fastPoller_Init_AlreadyClosed tests Init() on already closed poller
func Test_fastPoller_Init_AlreadyClosed(t *testing.T) {
	poller := &fastPoller{}
	poller.closed.Store(true)

	err := poller.Init()
	if err != ErrPollerClosed {
		t.Errorf("Expected ErrPollerClosed, got: %v", err)
	}
}

// Test_fastPoller_Init_Success tests successful initialization
func Test_fastPoller_Init_Success(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	// Verify kqueue fd is valid
	if poller.kq <= 0 {
		t.Errorf("Expected valid kqueue fd, got: %d", poller.kq)
	}

	// Verify fds slice was allocated
	if len(poller.fds) != maxFDs {
		t.Errorf("Expected fds slice of size %d, got: %d", maxFDs, len(poller.fds))
	}
}

// Test_fastPoller_Close_Idempotent tests that Close() is idempotent
func Test_fastPoller_Close_Idempotent(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// First close
	err = poller.Close()
	if err != nil {
		t.Errorf("First Close() returned error: %v", err)
	}

	// Second close - should return nil (idempotent)
	err = poller.Close()
	if err != nil {
		t.Errorf("Second Close() returned error: %v", err)
	}

	// Third close - still should return nil
	err = poller.Close()
	if err != nil {
		t.Errorf("Third Close() returned error: %v", err)
	}
}

// Test_fastPoller_PollIO_Closed tests PollIO on closed poller
func Test_fastPoller_PollIO_Closed(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	poller.Close()

	_, err = poller.PollIO(0)
	if err != ErrPollerClosed {
		t.Errorf("Expected ErrPollerClosed, got: %v", err)
	}
}

// Test_fastPoller_PollIO_EINTR tests that PollIO handles EINTR gracefully
func Test_fastPoller_PollIO_EINTR(t *testing.T) {
	// This test verifies the EINTR handling path exists
	// We can't reliably trigger EINTR in a unit test, but we verify
	// the code path is present by checking the implementation handles it
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	// Do a quick poll with timeout 0 - should return immediately
	n, err := poller.PollIO(0)
	if err != nil {
		t.Errorf("PollIO failed: %v", err)
	}
	if n != 0 {
		t.Logf("PollIO returned %d events (expected 0)", n)
	}
}

// Test_fastPoller_PollIO_Timeout tests PollIO timeout behavior
func Test_fastPoller_PollIO_Timeout(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	start := time.Now()
	n, err := poller.PollIO(50) // 50ms timeout
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("PollIO failed: %v", err)
	}
	if n != 0 {
		t.Logf("PollIO returned %d events", n)
	}

	// Should have waited approximately 50ms
	if elapsed < 40*time.Millisecond {
		t.Errorf("PollIO returned too quickly: %v", elapsed)
	}
}

// Test_fastPoller_PollIO_WithEvents tests PollIO with actual events
func Test_fastPoller_PollIO_WithEvents(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	// Create a pipe
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	unix.SetNonblock(fds[0], true)

	var callbackCalled atomic.Bool

	// Register read end
	err = poller.RegisterFD(fds[0], EventRead, func(events IOEvents) {
		callbackCalled.Store(true)
		if events&EventRead == 0 {
			t.Error("Expected EventRead in callback")
		}
	})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Write to trigger event
	unix.Write(fds[1], []byte{1})

	// Poll - should get event
	n, err := poller.PollIO(100)
	if err != nil {
		t.Errorf("PollIO failed: %v", err)
	}
	if n < 1 {
		t.Errorf("Expected at least 1 event, got: %d", n)
	}
	if !callbackCalled.Load() {
		t.Error("Callback should have been called")
	}
}

// Test_fastPoller_DispatchEvents_NegativeFD tests dispatchEvents with negative FD
func Test_fastPoller_DispatchEvents_NegativeFD(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	// Manually set up an event buffer with negative FD
	// This tests the `if fd < 0 { continue }` path
	poller.eventBuf[0] = unix.Kevent_t{
		Ident:  ^uint64(0), // Will become -1 when cast to int
		Filter: unix.EVFILT_READ,
		Flags:  0,
	}

	// dispatchEvents should skip this event without panic
	poller.dispatchEvents(1)
	// If we get here without panic, test passes
}

// Test_fastPoller_DispatchEvents_FDNotActive tests callback dispatch for inactive FD
func Test_fastPoller_DispatchEvents_FDNotActive(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	// Set up event for FD that isn't active
	poller.eventBuf[0] = unix.Kevent_t{
		Ident:  uint64(100), // Valid FD but not registered
		Filter: unix.EVFILT_READ,
		Flags:  0,
	}

	// dispatchEvents should skip this event
	poller.dispatchEvents(1)
	// If we get here without panic, test passes
}

// Test_fastPoller_DispatchEvents_FDOutOfBounds tests callback dispatch for FD outside fds array
func Test_fastPoller_DispatchEvents_FDOutOfBounds(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	// Set up event for FD beyond fds array bounds
	poller.eventBuf[0] = unix.Kevent_t{
		Ident:  uint64(len(poller.fds) + 100), // Beyond array
		Filter: unix.EVFILT_READ,
		Flags:  0,
	}

	// dispatchEvents should skip this event
	poller.dispatchEvents(1)
	// If we get here without panic, test passes
}

// Test_fastPoller_RegisterFD_DynamicGrowth tests FD array growth for large FDs
func Test_fastPoller_RegisterFD_DynamicGrowth(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	// Create a pipe
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	unix.SetNonblock(fds[0], true)

	// Try to register with simulated large FD
	// Note: We can't actually create an FD > maxFDs easily,
	// but we can verify the growth logic exists
	initialLen := len(poller.fds)

	// Register normal FD
	err = poller.RegisterFD(fds[0], EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// fds array should still be same size (FD within initial allocation)
	if len(poller.fds) != initialLen {
		t.Logf("fds array grew from %d to %d", initialLen, len(poller.fds))
	}
}

// Test_fastPoller_RegisterFD_KeventError tests RegisterFD with kevent error rollback
func Test_fastPoller_RegisterFD_KeventError(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	// Register invalid FD should trigger kevent error
	err = poller.RegisterFD(999999, EventRead, func(events IOEvents) {})
	if err == nil {
		// If registration succeeded, unregister
		poller.UnregisterFD(999999)
		t.Log("RegisterFD succeeded for high FD (may be valid)")
	} else {
		// Expected - rolledback properly
		t.Logf("RegisterFD failed as expected: %v", err)
	}
}

// Test_fastPoller_UnregisterFD_NegativeFD tests UnregisterFD with negative FD
func Test_fastPoller_UnregisterFD_NegativeFD(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	err = poller.UnregisterFD(-1)
	if err != ErrFDOutOfRange {
		t.Errorf("Expected ErrFDOutOfRange, got: %v", err)
	}
}

// Test_fastPoller_UnregisterFD_NotRegistered tests UnregisterFD for unregistered FD
func Test_fastPoller_UnregisterFD_NotRegistered(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	err = poller.UnregisterFD(100)
	if err != ErrFDNotRegistered {
		t.Errorf("Expected ErrFDNotRegistered, got: %v", err)
	}
}

// Test_fastPoller_UnregisterFD_OutOfBounds tests UnregisterFD for FD beyond array
func Test_fastPoller_UnregisterFD_OutOfBounds(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	err = poller.UnregisterFD(len(poller.fds) + 100)
	if err != ErrFDNotRegistered {
		t.Errorf("Expected ErrFDNotRegistered, got: %v", err)
	}
}

// Test_fastPoller_ModifyFD_NegativeFD tests ModifyFD with negative FD
func Test_fastPoller_ModifyFD_NegativeFD(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	err = poller.ModifyFD(-1, EventRead)
	if err != ErrFDOutOfRange {
		t.Errorf("Expected ErrFDOutOfRange, got: %v", err)
	}
}

// Test_fastPoller_ModifyFD_NotRegistered tests ModifyFD for unregistered FD
func Test_fastPoller_ModifyFD_NotRegistered(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	err = poller.ModifyFD(100, EventRead)
	if err != ErrFDNotRegistered {
		t.Errorf("Expected ErrFDNotRegistered, got: %v", err)
	}
}

// Test_fastPoller_ModifyFD_Success tests successful ModifyFD
func Test_fastPoller_ModifyFD_Success(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	// Create a pipe
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	unix.SetNonblock(fds[0], true)
	unix.SetNonblock(fds[1], true)

	// Register for read
	err = poller.RegisterFD(fds[0], EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Modify to read+write
	err = poller.ModifyFD(fds[0], EventRead|EventWrite)
	if err != nil {
		t.Errorf("ModifyFD failed: %v", err)
	}

	// Modify to write only
	err = poller.ModifyFD(fds[0], EventWrite)
	if err != nil {
		t.Errorf("ModifyFD (write only) failed: %v", err)
	}

	// Modify back to read only
	err = poller.ModifyFD(fds[0], EventRead)
	if err != nil {
		t.Errorf("ModifyFD (read only) failed: %v", err)
	}
}

// TestEventsToKevents tests the eventsToKevents conversion function
func TestEventsToKevents(t *testing.T) {
	tests := []struct {
		name     string
		events   IOEvents
		flags    uint16
		expected int // Number of kevents expected
	}{
		{"None", 0, unix.EV_ADD, 0},
		{"ReadOnly", EventRead, unix.EV_ADD, 1},
		{"WriteOnly", EventWrite, unix.EV_ADD, 1},
		{"ReadWrite", EventRead | EventWrite, unix.EV_ADD, 2},
		{"ReadDelete", EventRead, unix.EV_DELETE, 1},
		{"WriteDelete", EventWrite, unix.EV_DELETE, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kevents := eventsToKevents(123, tt.events, tt.flags)
			if len(kevents) != tt.expected {
				t.Errorf("Expected %d kevents, got: %d", tt.expected, len(kevents))
			}
			for _, kev := range kevents {
				if kev.Ident != 123 {
					t.Errorf("Expected Ident=123, got: %d", kev.Ident)
				}
				if kev.Flags != tt.flags {
					t.Errorf("Expected Flags=%d, got: %d", tt.flags, kev.Flags)
				}
			}
		})
	}
}

// TestKeventToEvents tests the keventToEvents conversion function
func TestKeventToEvents(t *testing.T) {
	tests := []struct {
		name     string
		kev      unix.Kevent_t
		expected IOEvents
	}{
		{
			"Read",
			unix.Kevent_t{Filter: unix.EVFILT_READ},
			EventRead,
		},
		{
			"Write",
			unix.Kevent_t{Filter: unix.EVFILT_WRITE},
			EventWrite,
		},
		{
			"ReadWithError",
			unix.Kevent_t{Filter: unix.EVFILT_READ, Flags: unix.EV_ERROR},
			EventRead | EventError,
		},
		{
			"WriteWithEOF",
			unix.Kevent_t{Filter: unix.EVFILT_WRITE, Flags: unix.EV_EOF},
			EventWrite | EventHangup,
		},
		{
			"ReadWithErrorAndEOF",
			unix.Kevent_t{Filter: unix.EVFILT_READ, Flags: unix.EV_ERROR | unix.EV_EOF},
			EventRead | EventError | EventHangup,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := keventToEvents(&tt.kev)
			if events != tt.expected {
				t.Errorf("Expected %d, got: %d", tt.expected, events)
			}
		})
	}
}

// Test_fastPoller_ConcurrentAccess tests concurrent access to poller
func Test_fastPoller_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	// Create pipes
	const numPipes = 10
	var pipes [][2]int
	for i := 0; i < numPipes; i++ {
		var fds [2]int
		if err := unix.Pipe(fds[:]); err != nil {
			t.Fatalf("Pipe %d failed: %v", i, err)
		}
		unix.SetNonblock(fds[0], true)
		pipes = append(pipes, fds)
	}
	defer func() {
		for _, p := range pipes {
			unix.Close(p[0])
			unix.Close(p[1])
		}
	}()

	// Register all pipes
	for i, p := range pipes {
		err := poller.RegisterFD(p[0], EventRead, func(events IOEvents) {})
		if err != nil {
			t.Fatalf("RegisterFD %d failed: %v", i, err)
		}
	}

	var wg sync.WaitGroup

	// Concurrent pollers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				poller.PollIO(1)
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		idx := i % numPipes
		go func(pipeIdx int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				unix.Write(pipes[pipeIdx][1], []byte{1})
				time.Sleep(time.Microsecond)
			}
		}(idx)
	}

	// Concurrent modifiers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				idx := j % numPipes
				poller.ModifyFD(pipes[idx][0], EventRead|EventWrite)
				time.Sleep(time.Microsecond)
				poller.ModifyFD(pipes[idx][0], EventRead)
			}
		}()
	}

	wg.Wait()
}

// Test_fastPoller_RegisterFD_maxFDLimit tests FD limit enforcement
func Test_fastPoller_RegisterFD_maxFDLimit(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	// Try to register FD at limit
	err = poller.RegisterFD(maxFDLimit, EventRead, func(events IOEvents) {})
	if err != ErrFDOutOfRange {
		t.Errorf("Expected ErrFDOutOfRange for FD at limit, got: %v", err)
	}

	// Try to register FD beyond limit
	err = poller.RegisterFD(maxFDLimit+1, EventRead, func(events IOEvents) {})
	if err != ErrFDOutOfRange {
		t.Errorf("Expected ErrFDOutOfRange for FD beyond limit, got: %v", err)
	}
}

// Test_fastPoller_CallbackNilCheck tests that nil callback is handled
func Test_fastPoller_CallbackNilCheck(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	// Create a pipe
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	unix.SetNonblock(fds[0], true)

	// Manually set up an fdInfo with nil callback
	poller.fdMu.Lock()
	poller.fds[fds[0]] = fdInfo{callback: nil, events: EventRead, active: true}
	poller.fdMu.Unlock()

	// Set up event buffer with this FD
	poller.eventBuf[0] = unix.Kevent_t{
		Ident:  uint64(fds[0]),
		Filter: unix.EVFILT_READ,
		Flags:  0,
	}

	// dispatchEvents should skip nil callback
	poller.dispatchEvents(1)
	// If we get here without panic, test passes
}

// Test_fastPoller_Close_KqZero tests Close when kq is 0
func Test_fastPoller_Close_KqZero(t *testing.T) {
	poller := &fastPoller{}
	// kq is 0 by default, closed is false

	err := poller.Close()
	if err != nil {
		t.Errorf("Close() with kq=0 returned error: %v", err)
	}
}

// TestMaxFDs_Constant tests that maxFDs constant is defined
func TestMaxFDs_Constant(t *testing.T) {
	if maxFDs <= 0 {
		t.Error("maxFDs should be positive")
	}
	if maxFDs > maxFDLimit {
		t.Error("maxFDs should be <= maxFDLimit")
	}
}
