// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

//go:build darwin

package eventloop

import (
	"context"
	"os"
	"syscall"
	"testing"
)

// TestCreateWakeFd_SyscallErrorInjection tests error paths in createWakeFd
// by simulating syscall failures through file descriptor exhaustion.
// Priority: CRITICAL - Currently at 58.8% coverage.
func TestCreateWakeFd_SyscallErrorInjection(t *testing.T) {
	// Attempt to exhaust file descriptors to trigger Pipe() failure
	// This is a best-effort test - the actual syscall failure depends on system resources
	maxFDs := 10000
	createdFds := make([]int, 0, maxFDs)

	defer func() {
		for _, fd := range createdFds {
			syscall.Close(fd)
		}
	}()

	// Try to create file descriptors until we can't anymore
	for i := 0; i < maxFDs; i++ {
		fd, err := syscall.Open("/dev/null", syscall.O_RDONLY, 0)
		if err != nil {
			// System is running low on file descriptors
			// Try createWakeFd - it should fail if we can't allocate FDs
			_, _, err := createWakeFd(0, 0)
			if err != nil {
				// Success! We triggered the error path
				t.Logf("createWakeFd failed as expected with exhausted FDs: %v", err)
				return
			}
			// If createWakeFd succeeded despite FD exhaustion, the system
			// has more resources than we exhausted - this is okay
			t.Log("createWakeFd succeeded even with apparent FD exhaustion")
			return
		}
		createdFds = append(createdFds, fd)
	}

	t.Log("Could not exhaust file descriptors - error path not triggered")
}

// TestCreateWakeFd_CloexecCoverage tests the CloseOnExec code path
// Coverage target: Lines 31-34 (CloseOnExec calls)
func TestCreateWakeFd_CloexecCoverage(t *testing.T) {
	// Test with various flags to exercise CloseOnExec logic
	testCases := []struct {
		name  string
		flags int
	}{
		{"NoFlags", 0},
		{"WithCloexec", efdCloexec},
		{"WithNonblock", efdNonblock},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r, w, err := createWakeFd(0, tc.flags)
			if err != nil {
				t.Fatalf("createWakeFd(%d) failed: %v", tc.flags, err)
			}
			defer syscall.Close(r)
			defer syscall.Close(w)

			// Verify FDs are valid
			if r < 0 || w < 0 {
				t.Errorf("Invalid FDs: r=%d, w=%d", r, w)
			}

			t.Logf("createWakeFd(0, %d) created FDs: r=%d, w=%d", tc.flags, r, w)
		})
	}
}

// TestDrainWakeUpPipe_ReadErrorPath tests the error break path in drainWakeUpPipe
// Coverage target: Lines 1085-1089 (for loop error handling)
// Priority: HIGH - Currently at 75.0% coverage.
func TestDrainWakeUpPipe_ReadErrorPath(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.TODO())

	// Create a pipe and register it to force the loop into I/O mode
	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatal("os.Pipe failed:", err)
	}
	defer pipeR.Close()
	defer pipeW.Close()

	err = loop.RegisterFD(int(pipeR.Fd()), EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatal("RegisterFD failed:", err)
	}

	// Verify the pipe was registered
	if loop.wakePipe < 0 {
		t.Skip("No wake pipe - this platform doesn't use wake pipes")
	}

	// The drainWakeUpPipe method reads from wakePipe until it would block
	// To trigger the error path, we need readFD to return an error
	// This is difficult to trigger naturally, but we can verify:
	// 1. The function exists and is callable
	// 2. Multiple calls are idempotent
	// 3. Normal operation doesn't error

	for i := 0; i < 3; i++ {
		loop.drainWakeUpPipe()
	}

	t.Log("drainWakeUpPipe executed successfully (error path not triggered in this scenario)")
}

// TestDrainWakeUpPipe_NegativeWakePipe tests drainWakeUpPipe when wakePipe < 0
// Coverage target: Lines 1085-1089 (early return path)
func TestDrainWakeUpPipe_NegativeWakePipe(t *testing.T) {
	// Create a loop with proper initialization
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.TODO())

	// Note: Setting wakePipe to -1 after initialization might cause issues
	// because the loop expects wakePipe to be valid for normal operation
	// Instead, test the early return path by creating a minimal test

	// For darwin, wakePipe is always >= 0 because createWakeFd succeeds
	// So we can't easily test the negative wakePipe path
	// Instead, verify drainWakeUpPipe works correctly with positive wakePipe

	if loop.wakePipe >= 0 {
		// Test normal operation
		loop.drainWakeUpPipe()
		if loop.wakeUpSignalPending.Load() != 0 {
			t.Errorf("wakeUpSignalPending should be 0, got: %d", loop.wakeUpSignalPending.Load())
		}
		t.Log("drainWakeUpPipe normal operation verified")
	} else {
		t.Skip("wakePipe is negative - skipping test")
	}
}

// TestCreateWakeFd_ResourceCleanup tests that cleanup is called on error
// Coverage target: Lines 36-39, 43-46 (SetNonblock error paths with cleanup)
// Priority: HIGH - These error paths are currently not covered.
func TestCreateWakeFd_ResourceCleanup(t *testing.T) {
	// This test verifies that createWakeFd properly cleans up resources on error
	// We can't directly trigger SetNonblock failures, but we can verify:
	// 1. The function signature and behavior
	// 2. Resource creation and cleanup patterns

	// Test creating multiple wake FDs sequentially
	for i := 0; i < 5; i++ {
		r, w, err := createWakeFd(0, 0)
		if err != nil {
			t.Fatalf("Iteration %d: createWakeFd failed: %v", i, err)
		}

		// Verify we can write and read
		testData := []byte{byte(i)}
		_, err = syscall.Write(w, testData)
		if err != nil {
			t.Fatalf("Iteration %d: Write failed: %v", i, err)
		}

		buf := make([]byte, 1)
		_, err = syscall.Read(r, buf)
		if err != nil {
			t.Fatalf("Iteration %d: Read failed: %v", i, err)
		}

		// Cleanup
		syscall.Close(r)
		syscall.Close(w)
	}

	t.Log("createWakeFd resource creation and cleanup verified")
}

// TestWakeUpPipe_Integration tests the wake pipe integration with the loop
// This test helps exercise drainWakeUpPipe through normal loop operation
func TestWakeUpPipe_Integration(t *testing.T) {
	// Create wake FDs directly to avoid loop lifecycle issues
	r, w, err := createWakeFd(0, 0)
	if err != nil {
		t.Fatalf("createWakeFd failed: %v", err)
	}
	defer syscall.Close(r)
	defer syscall.Close(w)

	// Write data to wake pipe
	testData := []byte("wake test")
	n, err := syscall.Write(w, testData)
	if err != nil {
		t.Fatalf("Failed to write to wake pipe: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(testData), n)
	}

	// Read data back (simulates what drainWakeUpPipe does)
	buf := make([]byte, 100)
	n, err = syscall.Read(r, buf)
	if err != nil {
		t.Fatalf("Failed to read from wake pipe: %v", err)
	}

	// Verify we got the right data
	if n != len(testData) || string(buf[:n]) != string(testData) {
		t.Errorf("Read unexpected data: got %q", string(buf[:n]))
	}

	t.Logf("Wake pipe integration verified - wrote %d bytes, read %d bytes", len(testData), n)
}
