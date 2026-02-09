// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

//go:build darwin

package eventloop

import (
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// COVERAGE-010: createWakeFd Full Coverage Per Platform (Darwin)
// Tests: Pipe creation, close-on-exec, non-blocking flags, error paths

// TestCreateWakeFd_Success tests successful pipe creation
func TestCreateWakeFd_Success(t *testing.T) {
	r, w, err := createWakeFd(0, efdCloexec|efdNonblock)
	if err != nil {
		t.Fatalf("createWakeFd failed: %v", err)
	}
	defer syscall.Close(r)
	defer syscall.Close(w)

	// Verify both FDs are valid
	if r < 0 {
		t.Errorf("Read FD should be >= 0, got: %d", r)
	}
	if w < 0 {
		t.Errorf("Write FD should be >= 0, got: %d", w)
	}

	// Verify read and write FDs are different (pipe semantics on Darwin)
	if r == w {
		t.Error("Read and write FDs should be different on Darwin (pipe)")
	}
}

// TestCreateWakeFd_NonBlocking tests that the pipe is non-blocking
func TestCreateWakeFd_NonBlocking(t *testing.T) {
	r, w, err := createWakeFd(0, efdCloexec|efdNonblock)
	if err != nil {
		t.Fatalf("createWakeFd failed: %v", err)
	}
	defer syscall.Close(r)
	defer syscall.Close(w)

	// Read from empty pipe should return EAGAIN (non-blocking)
	var buf [8]byte
	_, err = syscall.Read(r, buf[:])
	if err != syscall.EAGAIN {
		t.Errorf("Expected EAGAIN from empty non-blocking pipe, got: %v", err)
	}
}

// TestCreateWakeFd_WriteAndRead tests writing to and reading from the pipe
func TestCreateWakeFd_WriteAndRead(t *testing.T) {
	r, w, err := createWakeFd(0, efdCloexec|efdNonblock)
	if err != nil {
		t.Fatalf("createWakeFd failed: %v", err)
	}
	defer syscall.Close(r)
	defer syscall.Close(w)

	// Write to pipe
	data := []byte("testdata")
	n, err := syscall.Write(w, data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Expected to write %d bytes, wrote: %d", len(data), n)
	}

	// Read from pipe
	buf := make([]byte, 100)
	n, err = syscall.Read(r, buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Expected to read %d bytes, got: %d", len(data), n)
	}
	if string(buf[:n]) != string(data) {
		t.Errorf("Expected to read %q, got: %q", data, buf[:n])
	}
}

// TestCreateWakeFd_FlagsIgnored tests that initval and flags are ignored on Darwin
// (API compatibility with Linux eventfd)
func TestCreateWakeFd_FlagsIgnored(t *testing.T) {
	// createWakeFd ignores these parameters on Darwin
	// They exist for API compatibility with Linux eventfd
	r, w, err := createWakeFd(12345, 0xFFFF)
	if err != nil {
		t.Fatalf("createWakeFd failed (should ignore params): %v", err)
	}
	defer syscall.Close(r)
	defer syscall.Close(w)

	// Should still work despite strange parameters
	if r < 0 || w < 0 {
		t.Error("FDs should be valid despite ignored parameters")
	}
}

// TestCreateWakeFd_CloseOnExec tests that FDs have close-on-exec flag set
func TestCreateWakeFd_CloseOnExec(t *testing.T) {
	r, w, err := createWakeFd(0, efdCloexec|efdNonblock)
	if err != nil {
		t.Fatalf("createWakeFd failed: %v", err)
	}
	defer syscall.Close(r)
	defer syscall.Close(w)

	// Check close-on-exec flag on read FD
	flags, err := unix.FcntlInt(uintptr(r), unix.F_GETFD, 0)
	if err != nil {
		t.Fatalf("FcntlInt(F_GETFD) failed: %v", err)
	}
	if flags&unix.FD_CLOEXEC == 0 {
		t.Error("Read FD should have FD_CLOEXEC set")
	}

	// Check close-on-exec flag on write FD
	flags, err = unix.FcntlInt(uintptr(w), unix.F_GETFD, 0)
	if err != nil {
		t.Fatalf("FcntlInt(F_GETFD) failed: %v", err)
	}
	if flags&unix.FD_CLOEXEC == 0 {
		t.Error("Write FD should have FD_CLOEXEC set")
	}
}

// TestCreateWakeFd_NonBlockingFlag tests that FDs have non-blocking flag set
func TestCreateWakeFd_NonBlockingFlag(t *testing.T) {
	r, w, err := createWakeFd(0, efdCloexec|efdNonblock)
	if err != nil {
		t.Fatalf("createWakeFd failed: %v", err)
	}
	defer syscall.Close(r)
	defer syscall.Close(w)

	// Check non-blocking flag on read FD
	flags, err := unix.FcntlInt(uintptr(r), unix.F_GETFL, 0)
	if err != nil {
		t.Fatalf("FcntlInt(F_GETFL) failed: %v", err)
	}
	if flags&unix.O_NONBLOCK == 0 {
		t.Error("Read FD should have O_NONBLOCK set")
	}

	// Check non-blocking flag on write FD
	flags, err = unix.FcntlInt(uintptr(w), unix.F_GETFL, 0)
	if err != nil {
		t.Fatalf("FcntlInt(F_GETFL) failed: %v", err)
	}
	if flags&unix.O_NONBLOCK == 0 {
		t.Error("Write FD should have O_NONBLOCK set")
	}
}

// TestCreateWakeFd_MultipleCreation tests creating multiple wake pipes
func TestCreateWakeFd_MultipleCreation(t *testing.T) {
	var fds [][2]int

	// Create multiple pipes
	for i := 0; i < 10; i++ {
		r, w, err := createWakeFd(0, efdCloexec|efdNonblock)
		if err != nil {
			t.Fatalf("createWakeFd %d failed: %v", i, err)
		}
		fds = append(fds, [2]int{r, w})
	}

	// Clean up
	for _, fd := range fds {
		syscall.Close(fd[0])
		syscall.Close(fd[1])
	}

	t.Logf("Successfully created and closed %d wake pipes", len(fds))
}

// TestIsWakeFdSupported_Darwin tests that wake FD is supported on Darwin
func TestIsWakeFdSupported_Darwin(t *testing.T) {
	if !isWakeFdSupported() {
		t.Error("isWakeFdSupported should return true on Darwin")
	}
}

// TestDrainWakeUpPipeStub_Darwin tests the stub function
func TestDrainWakeUpPipeStub_Darwin(t *testing.T) {
	// drainWakeUpPipe is a stub on Darwin that returns nil
	// The actual implementation is loop.drainWakeUpPipe()
	err := drainWakeUpPipe()
	if err != nil {
		t.Errorf("drainWakeUpPipe stub should return nil, got: %v", err)
	}
}

// TestSubmitGenericWakeupStub_Darwin tests the stub function
func TestSubmitGenericWakeupStub_Darwin(t *testing.T) {
	// submitGenericWakeup is a stub on Darwin that returns nil
	// Darwin uses pipe-based wakeup, not generic wakeup
	err := submitGenericWakeup(0)
	if err != nil {
		t.Errorf("submitGenericWakeup stub should return nil, got: %v", err)
	}

	// Should also work with non-zero values (ignored)
	err = submitGenericWakeup(12345)
	if err != nil {
		t.Errorf("submitGenericWakeup stub should return nil, got: %v", err)
	}
}

// TestEFDConstants_Darwin tests that EFD constants are defined correctly
func TestEFDConstants_Darwin(t *testing.T) {
	// On Darwin, these map to O_CLOEXEC and O_NONBLOCK
	if efdCloexec != unix.O_CLOEXEC {
		t.Errorf("efdCloexec should equal O_CLOEXEC, got: %d vs %d", efdCloexec, unix.O_CLOEXEC)
	}
	if efdNonblock != unix.O_NONBLOCK {
		t.Errorf("efdNonblock should equal O_NONBLOCK, got: %d vs %d", efdNonblock, unix.O_NONBLOCK)
	}
}

// TestCreateWakeFd_UsedByLoop tests that createWakeFd is correctly used by loop.New()
func TestCreateWakeFd_UsedByLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	// Verify wake pipe was created
	if loop.wakePipe < 0 {
		t.Error("Loop wakePipe should be >= 0 on Darwin")
	}
	if loop.wakePipeWrite < 0 {
		t.Error("Loop wakePipeWrite should be >= 0 on Darwin")
	}

	// Verify they are different (pipe semantics)
	if loop.wakePipe == loop.wakePipeWrite {
		t.Error("wakePipe and wakePipeWrite should be different on Darwin")
	}
}

// TestCreateWakeFd_WakeupMechanism tests the full wakeup mechanism through the pipe
func TestCreateWakeFd_WakeupMechanism(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	// Write to wake pipe (simulate submitWakeup)
	var one uint64 = 1
	buf := make([]byte, 8)
	for i := 0; i < 8; i++ {
		buf[i] = byte(one >> (i * 8))
	}
	_, err = writeFD(loop.wakePipeWrite, buf)
	if err != nil {
		t.Fatalf("Write to wake pipe failed: %v", err)
	}

	// Read from wake pipe (verify data arrived)
	var readBuf [8]byte
	n, err := readFD(loop.wakePipe, readBuf[:])
	if err != nil {
		t.Fatalf("Read from wake pipe failed: %v", err)
	}
	if n != 8 {
		t.Errorf("Expected to read 8 bytes, got: %d", n)
	}
}
