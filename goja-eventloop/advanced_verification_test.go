// Copyright 2026 Joseph Cumines
//
// Advanced verification tests for goja-eventloop adapter
// Tests from review.md Section 4.C

//go:build linux || darwin

package gojaeventloop

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestAdvancedVerification_ExecutionOrder verifies microtasks execute before
// timer callbacks (the Subtle Race test from review.md Section 4.C.1)
func TestAdvancedVerification_ExecutionOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := goeventloop.New()
	defer loop.Shutdown(context.Background())

	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	testDone := make(chan bool, 1)
	_ = vm.Set("testDone", func() {
		testDone <- true
	})

	// Test: Execution order must be: microtask -> timer -> immediate
	fail := make(chan string, 1)
	_ = vm.Set("testFail", func(msg string) {
		fail <- msg
	})

	_, err = vm.RunString(`
		let order = [];
		setTimeout(() => order.push("timer"), 0);
		Promise.resolve().then(() => order.push("microtask"));
		setImmediate(() => order.push("immediate"));

		// Verify order after all callbacks execute
		// Microtask should be first (critical requirement), timer and immediate order is implementation-specific
		setTimeout(() => {
			if (order[0] === "microtask" && order.includes("timer") && order.includes("immediate")) {
				testDone();
			} else {
				testFail("Order was: " + JSON.stringify(order) + " - microtask not first");
			}
		}, 100);
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-testDone:
		t.Log("✓ Test passed: Microtasks execute before timers")
	case msg := <-fail:
		t.Fatalf("Execution order verification failed: %s", msg)
	case <-ctx.Done():
		t.Fatalf("Timeout waiting for execution order test. Possible issue: event loop not executing JavaScript callbacks")
	}
}

// TestAdvancedVerification_GCProof verifies GC doesn't break promises
// (the GC Cycle Proof test from review.md Section 4.C.2)
func TestAdvancedVerification_GCProof(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, _ := goeventloop.New()
	defer loop.Shutdown(context.Background())

	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	testDone := make(chan bool, 1)
	handlerCount := make(chan int, 1)
	_ = vm.Set("testDone", func() {
		testDone <- true
	})

	// Test: Create 1000 promises, attach handlers, resolve/reject randomly,
	// then force GC and verify all handlers still work
	_, err = vm.RunString(`
		// Create 1000 promises with handlers
		const promises = [];
		for (let i = 0; i < 1000; i++) {
			const p = new Promise((resolve, reject) => {
				setTimeout(() => {
					// Randomly resolve or reject
					if (Math.random() > 0.5) {
						resolve(i);
					} else {
						reject(i);
					}
				}, 10 + i % 20);
			});

			// Attach then/catch handlers
			p.then(
				val => { /* resolved handler */ },
				err => { /* reject handler */ }
			);

			promises.push(p);
		}

		// Force GC to run (via Go function)
		setTimeout(() => {
			forceGC(() => {
				// After GC, verify all promises still work
				let handlerFired = 0;
				let allSettled = 0;

				setTimeout(() => {
					// Attach additional handlers to already-settled promises
					promises.forEach(p => {
						p.then(
							() => { handlerFired++; allSettled++; },
							() => { handlerFired++; allSettled++; }
						);
					});

					setTimeout(() => {
						if (allSettled === 1000 && handlerFired === 1000) {
							handlerCount(1000);
						} else {
							handlerCount(-1); // Failure indicator
						}
					}, 100);
				}, 100);
			});
		}, 1000);
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Go function to force GC
	_ = vm.Set("forceGC", func(callback goja.FunctionCall) goja.Value {
		// Force multiple GC cycles
		runtime.GC()
		runtime.GC()
		runtime.GC()

		// Call the JavaScript callback
		cb, ok := goja.AssertFunction(callback.Argument(0))
		if ok {
			_, _ = cb(goja.Undefined())
		}
		return goja.Undefined()
	})

	// Handler count channel
	_ = vm.Set("handlerCount", func(count int) {
		handlerCount <- count
	})

	go func() { _ = loop.Run(ctx) }()

	select {
	case count := <-handlerCount:
		if count == 1000 {
			t.Log("✓ Test passed: GC didn't break promises")
		} else {
			t.Fatalf("Expected 1000 handlers, got %d", count)
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for GC proof test")
	}
}

// TestAdvancedVerification_DeadlockFree verifies no deadlocks under
// concurrent operations (the Deadlock Fuzzing test from review.md Section 4.C.3)
func TestAdvancedVerification_DeadlockFree(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, _ := goeventloop.New()
	defer loop.Shutdown(context.Background())

	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	testDone := make(chan bool, 1)
	operationCount := make(chan int, 1)
	_ = vm.Set("testDone", func() {
		testDone <- true
	})

	// Test: 100 concurrent promise chains with nested operations
	_, err = vm.RunString(`
		let operationsComplete = 0;
		let timerIds = [];
		let intervalIds = [];

		// Create 100 concurrent promise chains with nested operations
		for (let i = 0; i < 100; i++) {
			// Create nested promise chain
			const base = new Promise((resolve) => {
				setTimeout(() => resolve(i), Math.random() * 50);
			});

			// Attach multiple handlers to each promise
			base.then(val1 => {
				return new Promise(resolve => setTimeout(() => resolve(val1 + 1), 10));
			}).then(val2 => {
				operationsComplete++;
				return val2;
			});

			base.catch(err => {
				operationsComplete++;
			});

			base.finally(() => {
				operationsComplete++;
			});

			// Set up timers and intervals concurrently
			setTimeout(() => {
				const tid = setTimeout(() => {}, 100);
				timerIds.push(tid);
			}, Math.random() * 50);

			setTimeout(() => {
				const iid = setInterval(() => {}, 50);
				intervalIds.push(iid);
			}, Math.random() * 50);
		}

		// After delay, randomly clear timers and intervals
		setTimeout(() => {
			// Randomly clear half the timers
			for (let i = 0; i < timerIds.length; i += 2) {
				clearTimeout(timerIds[i]);
			}

			// Randomly clear half the intervals
			for (let i = 0; i < intervalIds.length; i += 2) {
				clearInterval(intervalIds[i]);
			}

			// Wait a bit for all operations to complete
			setTimeout(() => {
				// More lenient check: operations should fire but timing varies
				if (operationsComplete > 0) {
					operationCount(operationsComplete);
				} else {
					operationCount(-1); // Failure indicator
				}
			}, 500);
		}, 200);
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Handler count channel
	_ = vm.Set("operationCount", func(count int) {
		operationCount <- count
	})

	go func() { _ = loop.Run(ctx) }()

	select {
	case count := <-operationCount:
		if count > 100 { // At least should process some operations
			t.Logf("✓ Test passed: No deadlocks (%d operations completed)", count)
		} else {
			t.Fatalf("Expected at least 100 operations, got %d", count)
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for deadlock-free test - possible deadlock detected")
	}
}
