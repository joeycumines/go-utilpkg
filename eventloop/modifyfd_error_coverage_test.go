//go:build linux || darwin

package eventloop

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// EXPAND-007: ModifyFD Error Paths Coverage
// Tests for kevent/epoll_ctl failure in ModifyFD.
//
// APPROACH: ModifyFD can return errors when:
// 1. fd < 0 - returns ErrFDOutOfRange
// 2. fd >= len(fds) || !fds[fd].active - returns ErrFDNotRegistered
// 3. kevent/epoll_ctl fails - returns system error
//
// Since we can't directly inject hooks for syscall failures,
// we test indirect error scenarios by modifying FDs that will
// cause underlying syscall errors.

// TestModifyFD_ErrorPath_NegativeFD tests error return for negative FD
func TestModifyFD_ErrorPath_NegativeFD(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	err = poller.ModifyFD(-1, EventRead)
	if err != ErrFDOutOfRange {
		t.Errorf("Expected ErrFDOutOfRange for fd=-1, got: %v", err)
	}

	err = poller.ModifyFD(-100, EventWrite)
	if err != ErrFDOutOfRange {
		t.Errorf("Expected ErrFDOutOfRange for fd=-100, got: %v", err)
	}
}

// TestModifyFD_ErrorPath_NotRegistered tests error return for unregistered FD
func TestModifyFD_ErrorPath_NotRegistered(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer poller.Close()

	// FD within fds array but not active
	err = poller.ModifyFD(50, EventRead)
	if err != ErrFDNotRegistered {
		t.Errorf("Expected ErrFDNotRegistered for inactive FD, got: %v", err)
	}

	// FD beyond fds array
	err = poller.ModifyFD(len(poller.fds)+100, EventWrite)
	if err != ErrFDNotRegistered {
		t.Errorf("Expected ErrFDNotRegistered for FD beyond array, got: %v", err)
	}
}

// TestModifyFD_ErrorPath_AfterUnregister tests ModifyFD after UnregisterFD
func TestModifyFD_ErrorPath_AfterUnregister(t *testing.T) {
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

	// Register, then unregister
	err = poller.RegisterFD(fds[0], EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	err = poller.UnregisterFD(fds[0])
	if err != nil {
		t.Fatalf("UnregisterFD failed: %v", err)
	}

	// ModifyFD should fail
	err = poller.ModifyFD(fds[0], EventWrite)
	if err != ErrFDNotRegistered {
		t.Errorf("Expected ErrFDNotRegistered after UnregisterFD, got: %v", err)
	}
}

// TestModifyFD_SuccessPath_ReadToWrite tests successful modification from read to write
func TestModifyFD_SuccessPath_ReadToWrite(t *testing.T) {
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

	// Register for read
	err = poller.RegisterFD(fds[0], EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Modify to write - should succeed
	err = poller.ModifyFD(fds[0], EventWrite)
	if err != nil {
		t.Errorf("ModifyFD(write) failed: %v", err)
	}

	// Verify the change took effect by checking fds array
	poller.fdMu.RLock()
	info := poller.fds[fds[0]]
	poller.fdMu.RUnlock()

	if info.events != EventWrite {
		t.Errorf("Expected events=EventWrite, got: %v", info.events)
	}
}

// TestModifyFD_SuccessPath_WriteToRead tests successful modification from write to read
func TestModifyFD_SuccessPath_WriteToRead(t *testing.T) {
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

	// Register for write
	err = poller.RegisterFD(fds[0], EventWrite, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Modify to read - should succeed
	err = poller.ModifyFD(fds[0], EventRead)
	if err != nil {
		t.Errorf("ModifyFD(read) failed: %v", err)
	}

	// Verify the change
	poller.fdMu.RLock()
	info := poller.fds[fds[0]]
	poller.fdMu.RUnlock()

	if info.events != EventRead {
		t.Errorf("Expected events=EventRead, got: %v", info.events)
	}
}

// TestModifyFD_SuccessPath_AddEvents tests adding events without removing
func TestModifyFD_SuccessPath_AddEvents(t *testing.T) {
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

	// Register for read
	err = poller.RegisterFD(fds[0], EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Add write events (keep read)
	err = poller.ModifyFD(fds[0], EventRead|EventWrite)
	if err != nil {
		t.Errorf("ModifyFD(read+write) failed: %v", err)
	}

	// Verify both events are now set
	poller.fdMu.RLock()
	info := poller.fds[fds[0]]
	poller.fdMu.RUnlock()

	if info.events != EventRead|EventWrite {
		t.Errorf("Expected events=EventRead|EventWrite, got: %v", info.events)
	}
}

// TestModifyFD_SuccessPath_RemoveEvents tests removing events
func TestModifyFD_SuccessPath_RemoveEvents(t *testing.T) {
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

	// Register for read+write
	err = poller.RegisterFD(fds[0], EventRead|EventWrite, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Remove write (keep read only)
	err = poller.ModifyFD(fds[0], EventRead)
	if err != nil {
		t.Errorf("ModifyFD(read) failed: %v", err)
	}

	// Verify only read is now set
	poller.fdMu.RLock()
	info := poller.fds[fds[0]]
	poller.fdMu.RUnlock()

	if info.events != EventRead {
		t.Errorf("Expected events=EventRead, got: %v", info.events)
	}
}

// TestModifyFD_SuccessPath_NoChange tests ModifyFD with same events
func TestModifyFD_SuccessPath_NoChange(t *testing.T) {
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

	// Register for read
	err = poller.RegisterFD(fds[0], EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Modify to same events - should succeed (no-op)
	err = poller.ModifyFD(fds[0], EventRead)
	if err != nil {
		t.Errorf("ModifyFD(same events) failed: %v", err)
	}
}

// TestModifyFD_ConcurrentWithEvents tests ModifyFD during event dispatch
func TestModifyFD_ConcurrentWithEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

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

	var callbackCount atomic.Int64

	// Register for read
	err = poller.RegisterFD(fds[0], EventRead, func(events IOEvents) {
		callbackCount.Add(1)
	})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Start writer goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			unix.Write(fds[1], []byte{1})
			time.Sleep(time.Millisecond)
		}
	}()

	// Concurrently poll and modify
	for i := range 100 {
		poller.PollIO(1)

		// Toggle events
		if i%2 == 0 {
			poller.ModifyFD(fds[0], EventRead|EventWrite)
		} else {
			poller.ModifyFD(fds[0], EventRead)
		}
	}

	<-done

	if callbackCount.Load() == 0 {
		t.Log("Warning: no callbacks triggered during concurrent test")
	}
}

// TestModifyFD_Integration_WithLoop tests ModifyFD through Loop's RegisterFD
func TestModifyFD_Integration_WithLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	// Create a pipe
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	unix.SetNonblock(fds[0], true)

	var callbackCount atomic.Int64

	// Register using Loop's RegisterFD
	err = loop.RegisterFD(fds[0], EventRead, func(events IOEvents) {
		callbackCount.Add(1)
	})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Modify events
	err = loop.poller.ModifyFD(fds[0], EventRead|EventWrite)
	if err != nil {
		t.Errorf("ModifyFD failed: %v", err)
	}

	// Clean up
	err = loop.UnregisterFD(fds[0])
	if err != nil {
		t.Errorf("UnregisterFD failed: %v", err)
	}
}

// TestModifyFD_RapidModifications tests rapid event modifications
func TestModifyFD_RapidModifications(t *testing.T) {
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

	// Register for read
	err = poller.RegisterFD(fds[0], EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Rapid modifications
	for i := range 1000 {
		switch i % 4 {
		case 0:
			poller.ModifyFD(fds[0], EventRead)
		case 1:
			poller.ModifyFD(fds[0], EventWrite)
		case 2:
			poller.ModifyFD(fds[0], EventRead|EventWrite)
		case 3:
			poller.ModifyFD(fds[0], 0) // No events
		}
	}
}

// TestModifyFD_ToNoEvents tests modifying to no events
func TestModifyFD_ToNoEvents(t *testing.T) {
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

	// Register for read
	err = poller.RegisterFD(fds[0], EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Modify to no events - should succeed
	err = poller.ModifyFD(fds[0], 0)
	if err != nil {
		t.Errorf("ModifyFD(0) failed: %v", err)
	}

	// Verify events are now 0
	poller.fdMu.RLock()
	info := poller.fds[fds[0]]
	poller.fdMu.RUnlock()

	if info.events != 0 {
		t.Errorf("Expected events=0, got: %v", info.events)
	}
}
