//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"testing"

	"golang.org/x/sys/unix"
)

// ModifyFD Error Paths Coverage
// Tests common readiness-backend errors in ModifyFD.
//
// APPROACH: ModifyFD can return errors when:
// 1. fd < 0 - returns errFDNegative
// 2. fd >= len(fds) || !fds[fd].active - returns ErrFDNotRegistered
// 3. native mutation fails - returns system error
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
	registerPollerCleanupT(t, poller)

	err = poller.ModifyFD(-1, EventRead)
	if err != errFDNegative {
		t.Errorf("Expected errFDNegative for fd=-1, got: %v", err)
	}

	err = poller.ModifyFD(-100, EventWrite)
	if err != errFDNegative {
		t.Errorf("Expected errFDNegative for fd=-100, got: %v", err)
	}
}

// TestModifyFD_ErrorPath_NotRegistered tests error return for unregistered FD
func TestModifyFD_ErrorPath_NotRegistered(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	registerPollerCleanupT(t, poller)

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
	registerPollerCleanupT(t, poller)

	// Create a pipe
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	registerTestFDCleanupT(t, &fds[0], &fds[1])

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

func TestModifyFD_AfterCallerCloseUpdatesOwnedEvents(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	registerPollerCleanupT(t, poller)

	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("Socketpair failed: %v", err)
	}
	registerTestFDCleanupT(t, &fds[0], &fds[1])
	registeredFD := fds[0]
	if err := poller.RegisterFD(registeredFD, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}
	if err := unix.Close(registeredFD); err != nil {
		t.Fatalf("Close registered fd: %v", err)
	}
	fds[0] = -1

	if err := poller.ModifyFD(registeredFD, EventWrite); err != nil {
		t.Fatalf("ModifyFD through owned descriptor: %v", err)
	}
	poller.fdMu.RLock()
	info, active := poller.fdInfoLocked(registeredFD)
	poller.fdMu.RUnlock()
	if !active {
		t.Fatal("ModifyFD cleared owned registration")
	}
	if info.events != EventWrite {
		t.Fatalf("ModifyFD events = %v, want %v", info.events, EventWrite)
	}
	if err := poller.UnregisterFD(registeredFD); err != nil {
		t.Fatalf("UnregisterFD through owned descriptor: %v", err)
	}
}

func TestModifyFD_SparseAfterCallerCloseUpdatesOwnedEvents(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	registerPollerCleanupT(t, poller)

	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("Socketpair failed: %v", err)
	}
	registerTestFDCleanupT(t, &fds[0], &fds[1])

	highFD := duplicateSparseTestFD(t, fds[0])
	registerTestFDCleanupT(t, &highFD)
	registeredFD := highFD
	if err := poller.RegisterFD(registeredFD, EventWrite, func(IOEvents) {}); err != nil {
		closeTestFDT(t, &highFD)
		t.Fatalf("RegisterFD sparse fd failed: %v", err)
	}
	if err := unix.Close(highFD); err != nil {
		t.Fatalf("Close sparse fd: %v", err)
	}
	highFD = -1

	if err := poller.ModifyFD(registeredFD, EventRead); err != nil {
		t.Fatalf("ModifyFD through sparse owned descriptor: %v", err)
	}
	poller.fdMu.RLock()
	info, active := poller.fdInfoLocked(registeredFD)
	_, inSparse := poller.sparseFDs[registeredFD]
	poller.fdMu.RUnlock()
	if !active || !inSparse {
		t.Fatalf("ModifyFD sparse active=%v inSparse=%v, want retained sparse registration", active, inSparse)
	}
	if info.events != EventRead {
		t.Fatalf("ModifyFD sparse events = %v, want %v", info.events, EventRead)
	}
	if err := poller.UnregisterFD(registeredFD); err != nil {
		t.Fatalf("UnregisterFD through sparse owned descriptor: %v", err)
	}
}

// TestModifyFD_SuccessPath_ReadToWrite tests successful modification from read to write
func TestModifyFD_SuccessPath_ReadToWrite(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	registerPollerCleanupT(t, poller)

	// Create a pipe
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	registerTestFDCleanupT(t, &fds[0], &fds[1])
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
	registerPollerCleanupT(t, poller)

	// Create a pipe
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	registerTestFDCleanupT(t, &fds[0], &fds[1])
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
	registerPollerCleanupT(t, poller)

	// Create a pipe
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	registerTestFDCleanupT(t, &fds[0], &fds[1])
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
	registerPollerCleanupT(t, poller)

	// Create a pipe
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	registerTestFDCleanupT(t, &fds[0], &fds[1])
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
	registerPollerCleanupT(t, poller)

	// Create a pipe
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	registerTestFDCleanupT(t, &fds[0], &fds[1])
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

// TestModifyFD_Integration_WithLoop tests ModifyFD through Loop's RegisterFD
func TestModifyFD_Integration_WithLoop(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	// Create a pipe
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	registerTestFDCleanupT(t, &fds[0], &fds[1])
	unix.SetNonblock(fds[0], true)

	// Register using Loop's RegisterFD
	err := loop.RegisterFD(fds[0], EventRead, func(IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Modify events
	err = loop.ModifyFD(fds[0], EventRead|EventWrite)
	if err != nil {
		t.Errorf("ModifyFD failed: %v", err)
	}
	loop.poller.fdMu.RLock()
	info, active := loop.poller.fdInfoLocked(fds[0])
	loop.poller.fdMu.RUnlock()
	if !active || info.events != EventRead|EventWrite {
		t.Fatalf("public ModifyFD active=%v events=%v, want active read|write", active, info.events)
	}
	if err := loop.ModifyFD(fds[0], 0); err != nil {
		t.Fatalf("ModifyFD(fd, 0): %v", err)
	}
	loop.poller.fdMu.RLock()
	info, active = loop.poller.fdInfoLocked(fds[0])
	loop.poller.fdMu.RUnlock()
	if !active || info.events != 0 {
		t.Fatalf("public ModifyFD(fd, 0) active=%v events=%v, want active zero", active, info.events)
	}

	// Clean up
	err = loop.UnregisterFD(fds[0])
	if err != nil {
		t.Errorf("UnregisterFD failed: %v", err)
	}
}

// TestModifyFD_ToNoEvents tests modifying to no events
func TestModifyFD_ToNoEvents(t *testing.T) {
	poller := &fastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	registerPollerCleanupT(t, poller)

	// Create a pipe
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	registerTestFDCleanupT(t, &fds[0], &fds[1])
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
