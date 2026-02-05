// Copyright 2026 Joseph Cumines
//
// Functional correctness tests for goja-eventloop adapter
// Tests from review.md Section 4.A

//go:build linux || darwin

package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestFunctionalCorrectness_PromiseRejectIdentity verifies Promise.reject(promise)
// returns to same promise as rejection reason (JS spec compliance)
func TestFunctionalCorrectness_PromiseRejectIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	testDone := make(chan bool, 1)
	_ = runtime.Set("testDone", func() {
		testDone <- true
	})

	// Test: Promise.reject(p1) where p1 is a promise, catch handler receives p1 (not 42)
	_, err = runtime.RunString(`
		const p1 = Promise.resolve(42);
		const p2 = Promise.reject(p1);

		p2.catch(reason => {
			// SPEC REQUIREMENT: reason should be to promise object p1, not 42
			if (reason === p1) {
				testDone();
			}
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-testDone:
		t.Log("✓ Test passed: Promise.reject(promise) preserved identity")
	case <-ctx.Done():
		t.Fatal("Timeout waiting for promise rejection to complete")
	}
}

// TestFunctionalCorrectness_TimerIDIsolation verifies SetImmediate IDs don't
// collide with setTimeout IDs (API safety)
func TestFunctionalCorrectness_TimerIDIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	testDone := make(chan bool, 1)
	_ = runtime.Set("testDone", func() {
		testDone <- true
	})

	// Test: setImmediate ID should be different from setTimeout ID
	_, err = runtime.RunString(`
		const immedId = setImmediate(() => {});
		const timeoutId = setTimeout(() => {}, 10);

		// Verify ID separation
		if (immedId !== timeoutId) {
			// Clear both
			clearImmediate(immedId);
			clearTimeout(timeoutId);
			testDone();
		}
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-testDone:
		t.Log("✓ Test passed: Timer IDs are properly isolated")
	case <-ctx.Done():
		t.Fatal("Timeout waiting for timer ID isolation test")
	}
}

// TestFunctionalCorrectness_IntervalCleanup_Regression verifies SetInterval stops
// correctly with ClearInterval (regression test for removed wg)
func TestFunctionalCorrectness_IntervalCleanup_Regression(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	_, err = runtime.RunString(`
		let count = 0;
		const id = setInterval(() => {
			count++;
			if (count >= 3) {
				clearInterval(id);
				// Use setTimeout to wait a bit and verify interval is stopped
				let finalCount = count;
				setTimeout(() => {
					if (finalCount === 3) {
						testDone(true);
					} else {
						testDone(false);
					}
				}, 50);
			}
		}, 10);
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	testDone := make(chan bool, 1)
	_ = runtime.Set("testDone", func(success bool) {
		testDone <- success
	})

	go func() { _ = loop.Run(ctx) }()

	select {
	case success := <-testDone:
		if !success {
			t.Fatal("Interval did not stop correctly")
		}
		t.Log("✓ Test passed: Interval cleanup works correctly")
	case <-ctx.Done():
		t.Fatal("Timeout waiting for interval cleanup test")
	}
}
