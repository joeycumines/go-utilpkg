//go:build linux || darwin

// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// TestPollIO_ErrorRecovery tests that the event loop handles poller errors
// gracefully without causing a CPU death spiral.
// Priority: HIGH - Cover handlePollError function (currently 0.0%).
func TestPollIO_ErrorRecovery(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	// Create an eventfd or pipe for I/O testing
	// We'll register it, then cause it to be invalid to trigger poller error
	var pipeFds [2]int
	if err := syscall.Pipe(pipeFds[:]); err != nil {
		t.Skip("Cannot create pipe for testing:", err)
	}
	pipeR := pipeFds[0]
	pipeW := pipeFds[1]
	defer syscall.Close(pipeR)
	defer syscall.Close(pipeW)

	// Register FD with event loop
	var pollErrorCaptured atomic.Bool
	err = loop.RegisterFD(pipeR, EventRead, func(events IOEvents) {
		// This handler should not be called in error path
		pollErrorCaptured.Store(true)
	})
	if err != nil {
		t.Fatal("RegisterFD failed:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var executed atomic.Int64
	go loop.Run(ctx)

	// Give loop time to enter poll
	time.Sleep(20 * time.Millisecond)

	// Close the FD while registered - this should cause poller error on next iteration
	syscall.Close(pipeR)

	// Submit some tasks to verify loop continues operating
	for i := 0; i < 50; i++ {
		loop.Submit(func() {
			executed.Add(1)
		})
	}

	// Wait for loop to process
	time.Sleep(100 * time.Millisecond)
	cancel()

	count := executed.Load()
	if count == 0 {
		t.Fatalf("No tasks executed - loop may have died from poller error")
	}

	t.Logf("Poller error recovery test passed: %d tasks executed despite FD error", count)
}

// TestPollIO_ConcurrentMultipleFD errors tests error handling with multiple FDs.
// Priority: MEDIUM - Ensure robustness under concurrent FD operations.
func TestPollIO_ConcurrentMultipleFDErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	const numFDs = 5
	fds := make([]int, numFDs)
	for i := 0; i < numFDs; i++ {
		var pipeFds [2]int
		if err := syscall.Pipe(pipeFds[:]); err != nil {
			t.Fatalf("Pipe %d failed: %v", i, err)
		}
		fds[i] = pipeFds[0]
		defer syscall.Close(pipeFds[1])
		defer syscall.Close(pipeFds[0])
	}

	// Register all FDs
	for i, fd := range fds {
		err := loop.RegisterFD(fd, EventRead, func(events IOEvents) {})
		if err != nil {
			t.Fatalf("RegisterFD %d failed: %v", i, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	var executed atomic.Int64
	go loop.Run(ctx)

	// Give loop time to enter poll
	time.Sleep(20 * time.Millisecond)

	// Close half the FDs concurrently
	var wg sync.WaitGroup
	for i := 0; i < numFDs/2; i++ {
		wg.Add(1)
		go func(fd int) {
			defer wg.Done()
			syscall.Close(fd)
		}(fds[i])
	}

	// Submit tasks concurrently
	for i := 0; i < 100; i++ {
		loop.Submit(func() {
			executed.Add(1)
		})
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)
	cancel()

	count := executed.Load()
	if count < 50 {
		t.Fatalf("Too few tasks executed (%d) - loop may be degraded from FD errors", count)
	}

	t.Logf("Multiple FD error test passed: %d tasks executed despite concurrent errors", count)
}
