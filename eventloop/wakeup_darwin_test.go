// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

//go:build darwin

package eventloop

import (
	"os"
	"testing"
)

// TestDarwinWakeup_DrainWakeUpPipe tests that drainWakeUpPipe
// is a stub that delegates to loop.drainWakeUpPipe().
// Priority: LOW - Stub function coverage (delegates to loop method).
func TestDarwinWakeup_DrainWakeUpPipe(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	// Verify drainWakeUpPipe returns nil (stub implementation)
	err := drainWakeUpPipe()
	if err != nil {
		t.Errorf("drainWakeUpPipe should return nil (stub), got: %v", err)
	}

	t.Log("drainWakeUpPipe stub verified - delegates to loop method")
}

// TestDarwinWakeup_IsWakeFdSupported tests that
// isWakeFdSupported returns true on Darwin.
// Priority: LOW - Stub function coverage (simple bool return).
func TestDarwinWakeup_IsWakeFdSupported(t *testing.T) {
	// Verify isWakeFdSupported returns true on Darwin
	supported := isWakeFdSupported()
	if !supported {
		t.Error("isWakeFdSupported should return true on Darwin")
	}

	t.Log("isWakeFdSupported verified - returns true on Darwin")
}

// TestDarwinWakeup_SubmitGenericWakeup tests that
// submitGenericWakeup is a stub that returns nil.
// Priority: LOW - Stub function coverage (compatibility shim).
func TestDarwinWakeup_SubmitGenericWakeup(t *testing.T) {
	// verify submitGenericWakeup returns nil (stub implementation)
	err := submitGenericWakeup(0)
	if err != nil {
		t.Errorf("submitGenericWakeup should return nil (stub), got: %v", err)
	}

	t.Log("submitGenericWakeup stub verified - returns nil on Darwin")
}

// TestDarwinWakeup_CreateWakeFd tests the pipe creation
// function used for wake-up notifications.
// Priority: MEDIUM - Core infrastructure function.
func TestDarwinWakeup_CreateWakeFd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode - allocates system resources")
	}

	// Test creating wake pipe
	r, w, err := createWakeFd(0, 0)
	if err != nil {
		t.Fatalf("createWakeFd failed: %v", err)
	}
	defer os.NewFile(uintptr(r), "pipe-read").Close()
	defer os.NewFile(uintptr(w), "pipe-write").Close()

	// Verify pipe ends are valid fds (>= 0 on Unix systems)
	if r < 0 {
		t.Errorf("read fd should be >= 0, got: %d", r)
	}
	if w < 0 {
		t.Errorf("write fd should be >= 0, got: %d", w)
	}

	t.Log("createWakeFd verified - creates pipe fds successfully")

	// Test that we can write to and read from the pipe
	data := []byte("test data")
	_, err = os.NewFile(uintptr(w), "pipe-write").Write(data)
	if err != nil {
		t.Errorf("Write to wake pipe failed: %v", err)
	}

	buf := make([]byte, 100)
	n, err := os.NewFile(uintptr(r), "pipe-read").Read(buf)
	if err != nil {
		t.Errorf("Read from wake pipe failed: %v", err)
	}

	if n != len(data) {
		t.Errorf("Expected to read %d bytes, got: %d", len(data), n)
	}
}
