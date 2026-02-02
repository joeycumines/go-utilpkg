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

// TestWake_Explicit tests explicit Wake() calls.
// Priority: HIGH - Wake() currently at 0.0% coverage.
func TestWake_Explicit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	// Wait for loop to enter sleep state
	time.Sleep(50 * time.Millisecond)

	// Call Wake() explicitly
	err = loop.Wake()
	if err != nil {
		t.Fatalf("Wake() returned error: %v", err)
	}

	// Wake() should not cause any issues
	t.Log("Wake() called successfully")
}

// TestWake_Concurrent tests concurrent Wake() calls.
// Priority: HIGH - Wake() thread-safety verification.
func TestWake_Concurrent(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Call Wake() from multiple goroutines
	var callCount atomic.Int32
	const numCalls = 100
	for i := 0; i < numCalls; i++ {
		go func() {
			err := loop.Wake()
			if err != nil {
				t.Errorf("Wake() returned error: %v", err)
			}
			callCount.Add(1)
		}()
	}

	// Wait for all calls
	time.Sleep(100 * time.Millisecond)

	count := callCount.Load()
	if count != numCalls {
		t.Errorf("Expected %d Wake() calls, got %d", numCalls, count)
	}

	t.Logf("Concurrent Wake() test passed: %d calls executed", count)
}

// TestWake_WithPendingWork tests Wake() with pending work.
// Priority: MEDIUM - Wake() behavior with queued tasks.
func TestWake_WithPendingWork(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var executed atomic.Int32

	go loop.Run(ctx)

	// Submit task without running
	err = loop.Submit(func() {
		executed.Add(1)
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Explicit Wake() to drain queue
	err = loop.Wake()
	if err != nil {
		t.Fatalf("Wake() failed: %v", err)
	}

	// Wait for execution
	time.Sleep(50 * time.Millisecond)

	count := executed.Load()
	if count == 0 {
		t.Error("Task did not execute after Wake()")
	}

	t.Logf("Wake() with pending work test passed: %d tasks executed", count)
}

// TestWake_AfterShutdown tests Wake() after loop shutdown.
// Priority: MEDIUM - Wake() error handling.
func TestWake_AfterShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	go loop.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	// Shutdown loop
	err = loop.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	cancel()
	time.Sleep(20 * time.Millisecond)

	// Call Wake() after shutdown
	err = loop.Wake()
	// Wake() after shutdown should either succeed or return error gracefully
	// It should not panic
	if err != nil {
		// Error is acceptable after shutdown
		t.Logf("Wake() after shutdown returned error (acceptable): %v", err)
	} else {
		t.Log("Wake() after shutdown succeeded (also acceptable)")
	}
}
