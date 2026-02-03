// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestWake_Direct tests calling Wake() method.
// Priority: CRITICAL - Wake() currently at 0.0% coverage.
//
// This is the public API function that external callers use to wake up
// the event loop after submitting work.
func TestWake_Direct(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	// Wait for loop to be running
	time.Sleep(30 * time.Millisecond)

	// Call Wake() explicitly - this should wake the loop from sleeping
	err = loop.Wake()
	if err != nil {
		t.Fatalf("Wake() returned error: %v", err)
	}

	t.Log("Wake() direct call test passed")
}

// TestWake_FromExternalThread tests Wake() from outside loop goroutine.
// Priority: HIGH - Concurrency verification.
func TestWake_FromExternalThread(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	// Wait for loop to be running
	time.Sleep(30 * time.Millisecond)

	// Call Wake() from external goroutine
	go func() {
		err := loop.Wake()
		if err != nil {
			t.Error("Wake() failed from external thread:", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	t.Log("Wake() from external thread test passed")
}

// TestWake_WithPendingTasks tests Wake() with tasks in queue.
// Priority: HIGH - Integration with Submit().
func TestWake_WithPendingTasks(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var executed atomic.Int32

	go loop.Run(ctx)

	// Submit tasks before calling Wake()
	for i := 0; i < 10; i++ {
		err := loop.Submit(func() {
			executed.Add(1)
		})
		if err != nil {
			t.Fatal("Submit failed:", err)
		}
	}

	// Call Wake() to ensure tasks are processed
	err = loop.Wake()
	if err != nil {
		t.Fatalf("Wake() returned error: %v", err)
	}

	// Wait for execution
	time.Sleep(100 * time.Millisecond)

	count := executed.Load()
	if count == 0 {
		t.Error("No tasks executed after Wake()")
	}

	t.Logf("Wake() with pending tasks test passed: %d tasks executed", count)
}

// TestWake_MultipleConcurrent tests concurrent Wake() calls.
// Priority: HIGH - Stress test.
func TestWake_MultipleConcurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	var wakeCount atomic.Int32
	go loop.Run(ctx)

	// Wait for loop to be running
	time.Sleep(20 * time.Millisecond)

	// Call Wake() from multiple goroutines concurrently
	const numCalls = 50
	for i := 0; i < numCalls; i++ {
		go func() {
			err := loop.Wake()
			if err != nil {
				t.Error("Wake() failed:", err)
			}
			wakeCount.Add(1)
		}()
	}

	// Wait for all wake calls
	time.Sleep(200 * time.Millisecond)

	count := wakeCount.Load()
	if count != numCalls {
		t.Errorf("Expected %d Wake() calls, got: %d", numCalls, count)
	}

	t.Logf("Wake() concurrent test passed: %d calls succeeded", count)
}
