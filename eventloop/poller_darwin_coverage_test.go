// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"testing"

	"golang.org/x/sys/unix"
)

// TestFastPoller_Wakeup tests the FastPoller.Wakeup() method
// This is a stub on Darwin that returns nil
func TestFastPoller_Wakeup(t *testing.T) {
	poller := &FastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatal(err)
	}
	defer poller.Close()

	// Wakeup is a stub on Darwin that returns nil
	err = poller.Wakeup()
	if err != nil {
		t.Errorf("Wakeup() returned error: %v", err)
	}
}

// TestFastPoller_Wakeup_AfterClose tests Wakeup() after poller is closed
func TestFastPoller_Wakeup_AfterClose(t *testing.T) {
	poller := &FastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatal(err)
	}

	// Close the poller
	err = poller.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Wakeup after close should still return nil (stub)
	err = poller.Wakeup()
	if err != nil {
		t.Errorf("Wakeup() after Close() returned error: %v", err)
	}
}

// TestFastPoller_Close tests the FastPoller.Close() method
func TestFastPoller_Close(t *testing.T) {
	poller := &FastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatal(err)
	}

	// Close the poller
	err = poller.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Double close should be safe
	err = poller.Close()
	if err != nil {
		t.Errorf("Double Close() returned error: %v", err)
	}
}

// TestFastPoller_RegisterFD tests FD registration
func TestFastPoller_RegisterFD(t *testing.T) {
	poller := &FastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatal(err)
	}
	defer poller.Close()

	// Create a pipe for testing
	r, w, err := createWakeFd(0, 0)
	if err != nil {
		t.Fatalf("createWakeFd failed: %v", err)
	}
	defer unix.Close(r)
	defer unix.Close(w)

	// Register the read end for reading
	err = poller.RegisterFD(r, EventRead, func(events IOEvents) {
	})
	if err != nil {
		t.Errorf("RegisterFD failed: %v", err)
	}

	// Unregister the FD
	err = poller.UnregisterFD(r)
	if err != nil {
		t.Errorf("UnregisterFD failed: %v", err)
	}
}

// TestFastPoller_RegisterFD_Closed tests FD registration on closed poller
func TestFastPoller_RegisterFD_Closed(t *testing.T) {
	poller := &FastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatal(err)
	}

	// Close the poller first
	poller.Close()

	// Try to register FD on closed poller
	err = poller.RegisterFD(0, EventRead, func(events IOEvents) {
	})
	if err != ErrPollerClosed {
		t.Errorf("Expected ErrPollerClosed, got: %v", err)
	}
}

// TestFastPoller_RegisterFD_OutOfRange tests FD registration with invalid FD
func TestFastPoller_RegisterFD_OutOfRange(t *testing.T) {
	poller := &FastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatal(err)
	}
	defer poller.Close()

	// Try to register with invalid FD
	err = poller.RegisterFD(-1, EventRead, func(events IOEvents) {
	})
	if err != ErrFDOutOfRange {
		t.Errorf("Expected ErrFDOutOfRange, got: %v", err)
	}
}

// TestFastPoller_RegisterFD_AlreadyRegistered tests FD registration for already registered FD
func TestFastPoller_RegisterFD_AlreadyRegistered(t *testing.T) {
	poller := &FastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatal(err)
	}
	defer poller.Close()

	// Create a pipe
	r, w, err := createWakeFd(0, 0)
	if err != nil {
		t.Fatalf("createWakeFd failed: %v", err)
	}
	defer unix.Close(r)
	defer unix.Close(w)

	// Register once
	err = poller.RegisterFD(r, EventRead, func(events IOEvents) {
	})
	if err != nil {
		t.Errorf("First RegisterFD failed: %v", err)
	}

	// Try to register again
	err = poller.RegisterFD(r, EventRead, func(events IOEvents) {
	})
	if err != ErrFDAlreadyRegistered {
		t.Errorf("Expected ErrFDAlreadyRegistered, got: %v", err)
	}
}

// TestFastPoller_Kqueue tests kqueue creation
func TestFastPoller_Kqueue(t *testing.T) {
	kq, err := unix.Kqueue()
	if err != nil {
		t.Skip("Kqueue not available")
	}
	defer unix.Close(kq)

	if kq < 0 {
		t.Error("Expected valid kqueue fd")
	}
}

// TestFastPoller_Kevent tests kevent system call
func TestFastPoller_Kevent(t *testing.T) {
	// Create a kqueue
	kq, err := unix.Kqueue()
	if err != nil {
		t.Skip("Kqueue not available")
	}
	defer unix.Close(kq)

	// Create a pipe
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	// Register for read events
	change := unix.Kevent_t{
		Ident:  uint64(fds[0]),
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_ADD | unix.EV_ENABLE,
	}
	changes := []unix.Kevent_t{change}

	// First call: register the event (changelist only)
	n, err := unix.Kevent(kq, changes, nil, nil)
	if err != nil {
		t.Fatalf("Kevent registration failed: %v", err)
	}
	if n != 0 {
		t.Logf("Kevent registration returned %d (expected 0 for changelist)", n)
	}

	// Write to the pipe to trigger the read event
	if _, err := unix.Write(fds[1], []byte{0}); err != nil {
		t.Fatalf("Write to pipe failed: %v", err)
	}

	// Second call: read events (empty changelist, output list provided)
	var events [1]unix.Kevent_t
	n, err = unix.Kevent(kq, nil, events[:], nil)
	if err != nil {
		t.Fatalf("Kevent poll failed: %v", err)
	}
	if n != 1 {
		t.Errorf("Expected 1 event, got: %d", n)
	}
}

// TestIOEvents_Constants tests IOEvents constants
func TestIOEvents_Constants(t *testing.T) {
	// EventRead = 1 << 0 = 1
	if EventRead != 1 {
		t.Errorf("Expected EventRead=1, got: %d", EventRead)
	}
	// EventWrite = 1 << 1 = 2
	if EventWrite != 2 {
		t.Errorf("Expected EventWrite=2, got: %d", EventWrite)
	}
}

// TestErrPollerClosed tests ErrPollerClosed error
func TestErrPollerClosed(t *testing.T) {
	if ErrPollerClosed.Error() != "eventloop: poller closed" {
		t.Errorf("Unexpected error message: %s", ErrPollerClosed.Error())
	}
}

// TestErrFDOutOfRange tests ErrFDOutOfRange error
func TestErrFDOutOfRange(t *testing.T) {
	if ErrFDOutOfRange.Error() != "eventloop: fd out of range (max 100000000)" {
		t.Errorf("Unexpected error message: %s", ErrFDOutOfRange.Error())
	}
}

// TestErrFDAlreadyRegistered tests ErrFDAlreadyRegistered error
func TestErrFDAlreadyRegistered(t *testing.T) {
	if ErrFDAlreadyRegistered.Error() != "eventloop: fd already registered" {
		t.Errorf("Unexpected error message: %s", ErrFDAlreadyRegistered.Error())
	}
}

// TestFastPoller_PollIO tests PollIO method
func TestFastPoller_PollIO(t *testing.T) {
	poller := &FastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatal(err)
	}
	defer poller.Close()

	// Create a pipe for testing
	r, w, err := createWakeFd(0, 0)
	if err != nil {
		t.Fatalf("createWakeFd failed: %v", err)
	}
	defer unix.Close(r)
	defer unix.Close(w)

	// Register the read end
	err = poller.RegisterFD(r, EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Poll with timeout (should return immediately since nothing to read)
	_, err = poller.PollIO(0)
	if err != nil {
		t.Errorf("PollIO failed: %v", err)
	}

	// Write to wake up the poller
	unix.Write(w, []byte{1})

	// Poll again - should get event
	_, err = poller.PollIO(100)
	if err != nil {
		t.Errorf("PollIO with data failed: %v", err)
	}
}

// TestFastPoller_ModifyFD tests ModifyFD method
func TestFastPoller_ModifyFD(t *testing.T) {
	poller := &FastPoller{}
	err := poller.Init()
	if err != nil {
		t.Fatal(err)
	}
	defer poller.Close()

	// Create a pipe
	r, w, err := createWakeFd(0, 0)
	if err != nil {
		t.Fatalf("createWakeFd failed: %v", err)
	}
	defer unix.Close(r)
	defer unix.Close(w)

	// Register for read
	err = poller.RegisterFD(r, EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Modify to also write
	err = poller.ModifyFD(r, EventRead|EventWrite)
	if err != nil {
		t.Errorf("ModifyFD failed: %v", err)
	}
}
