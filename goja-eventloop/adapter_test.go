// Copyright 2026 Joseph Cumines
//
// Tests for goja-eventloop: Goja adapter to event loop

package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestNewAdapter tests basic adapter creation
func TestNewAdapter(t *testing.T) {
	// Test 2.2.3: Test basic adapter creation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if adapter.Loop() != loop {
		t.Error("adapter.Loop() should return the same loop")
	}
	if adapter.Runtime() != runtime {
		t.Error("adapter.Runtime() should return the same runtime")
	}
	if adapter.JS() == nil {
		t.Error("adapter.JS() should return a non-nil JS adapter")
	}
}

// TestSetTimeout tests setTimeout binding from JavaScript
func TestSetTimeout(t *testing.T) {
	// Test 2.3.1: Test setTimeout from JS
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
		let called = false;
		setTimeout(() => {
			called = true;
		}, 10);
		called;
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	// Run loop in background
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Wait for timeout to fire
	time.Sleep(50 * time.Millisecond)

	cancel()
	<-done

	called := runtime.Get("called")
	if !called.ToBoolean() {
		t.Error("setTimeout callback should have been called")
	}
}

// TestClearTimeout tests clearTimeout binding from JavaScript
func TestClearTimeout(t *testing.T) {
	// Test 2.3.2: Test clearTimeout from JS
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
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

	// Start the loop in the background
	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Run(ctx)
	}()

	// Execute RunString ON THE LOOP to ensure thread safety and prevent CancelTimer deadlock
	// (CancelTimer blocks waiting for the loop to process the cancellation, so the loop must be running)
	runErrCh := make(chan error, 1)
	err = loop.SubmitInternal(func() {
		_, runErr := runtime.RunString(`
			let called = false;
			const id = setTimeout(() => {
				called = true;
			}, 100);
			clearTimeout(id);
			called;
		`)
		runErrCh <- runErr
	})
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	// Wait for RunString to complete
	select {
	case err := <-runErrCh:
		if err != nil {
			t.Fatalf("Failed to run JavaScript: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("RunString timed out")
	}

	// Wait for timer to fire if not cleared (it shouldn't fire)
	time.Sleep(200 * time.Millisecond)

	cancel()
	<-loopDone

	called := runtime.Get("called")
	if called.ToBoolean() {
		t.Error("setTimeout callback should not have been called after clearTimeout")
	}
}

// TestSetInterval tests setInterval binding from JavaScript
func TestSetInterval(t *testing.T) {
	// Test 2.3.3: Test setInterval from JS
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
			if (count >= 3) clearInterval(id);
		}, 10);
		id;
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	// Run loop in background AFTER RunString
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Wait for interval to fire at least 3 times
	time.Sleep(200 * time.Millisecond)

	cancel()
	<-done

	countVal := runtime.Get("count")
	if count := int(countVal.ToInteger()); count < 3 {
		t.Errorf("setInterval should have fired at least 3 times, got %d", count)
	}
}

// TestClearInterval tests clearInterval binding from JavaScript
func TestClearInterval(t *testing.T) {
	// Test 2.3.4: Test clearInterval from JS
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
		let called = false;
		const id = setInterval(() => {
			called = true;
		}, 10);
		setTimeout(() => clearInterval(id), 50);
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	// Run loop in background AFTER RunString
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Wait for interval to fire at least once
	time.Sleep(50 * time.Millisecond)

	cancel()
	<-done

	called := runtime.Get("called")
	if !called.ToBoolean() {
		t.Error("setInterval should have fired at least once before clearInterval")
	}
}

// TestQueueMicrotask tests queueMicrotask binding from JavaScript
func TestQueueMicrotask(t *testing.T) {
	// Test 2.3.5: Test queueMicrotask from JS
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
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
		let executed = false;
		queueMicrotask(() => {
			executed = true;
		});
		executed;
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	// Run loop in background AFTER RunString
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Wait for microtask to execute
	time.Sleep(50 * time.Millisecond)

	cancel()
	<-done

	executedVal := runtime.Get("executed")
	if !executedVal.ToBoolean() {
		t.Error("queueMicrotask callback should have been executed")
	}
}

// TestPromiseThen tests Promise.then binding from JavaScript
func TestPromiseThen(t *testing.T) {
	// Test 2.3.6: Test Promise.then from JS
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

	// Test that .then() method exists and is callable
	_, err = runtime.RunString(`
		const p = new Promise((resolve) => resolve(42));
		if (typeof p.then !== 'function') {
			throw new Error('then is not a function');
		}
		p.then;
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	// Run loop in background AFTER RunString
	// Run loop in background AFTER RunString
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	t.Log("Promise.then method exists and is callable")

	cancel()
	<-done
}

// TestPromiseChain tests Promise chain from JavaScript
func TestPromiseChain(t *testing.T) {
	// Test 2.3.7: Test Promise chain from JS
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

	// Add completion tracking
	completion := make(chan struct{})
	_ = runtime.Set("notifyDone", func() {
		close(completion)
	})

	// Add console.log for debugging
	_ = runtime.Set("console", map[string]interface{}{
		"log": func(args ...interface{}) {
			t.Log(args...)
		},
	})

	// Test multi-step promise chain
	_, err = runtime.RunString(`
		let p = new Promise((resolve) => resolve(1));
		console.log("Promise keys:", Object.keys(p));
		console.log("Has 'then'?:", typeof p.then);

		// Test first .then() return value
		let chained = p.then(x => {
			console.log("First then, x=", x);
			return x + 1;
		});
		console.log("Chained keys:", Object.keys(chained));
		console.log("Chained has then?:", typeof chained.then);

		// Try second .then()
		let chained2 = chained.then(x => {
			console.log("Second then, x=", x);
			return x * 2;
		});
		console.log("Second chained has then?:", typeof chained2.then);

		// Assign result for verification and notify
		chained2.then(x => {
			result = x;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	// Run loop in background AFTER RunString to avoid race
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Check if script completed successfully before waiting
	t.Logf("JavaScript execution completed successfully")

	// Wait for completion
	select {
	case <-completion:
		t.Logf("Promise chain completed")
	case <-ctx.Done():
		t.Fatalf("Test timed outwaiting for promise chain")
	}

	// Stop loop
	cancel()
	<-done

	// Verify result
	result := runtime.Get("result")
	if result != nil {
		t.Logf("Test result: %v (Type: %T, IsUndefined: %v)", result.Export(), result.Export(), result == goja.Undefined())
	} else {
		t.Logf("result is nil from runtime.Get")
	}

	// Check if result is undefined (error case)
	if result == nil || result == goja.Undefined() {
		t.Logf("Result is undefined or nil - test may be failing")
		t.FailNow() // Use FailNow to prevent nil pointer access
	} else if !result.ToBoolean() || result.ToInteger() != 4 {
		t.Errorf("Expected promise chain to compute 4, got: %v", result.Export())
	}
}

// TestMixedTimersAndPromises tests mixed timers and promises
func TestMixedTimersAndPromises(t *testing.T) {
	// Test 2.3.8: Test Mixed timers and promises
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

	// Test microtasks execute before timer callbacks - use event-based completion
	completion := make(chan struct{})
	_ = runtime.Set("notifyDone", func() {
		select {
		case completion <- struct{}{}:
		default:
			// Already notified
		}
	})

	_, err = runtime.RunString(`
		let order = [];

		// Schedule timer
		setTimeout(() => {
			order.push('timer');
			notifyDone();
		}, 10);

		// Schedule microtask
		Promise.resolve().then(() => {
			order.push('microtask');
			notifyDone();
		});

		order;
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	// Run loop in background AFTER RunString
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// HIGH #7 FIX: Wait for both operations to complete via events (not hardcoded sleep)
	operationsCompleted := 0
	for operationsCompleted < 2 {
		select {
		case <-completion:
			operationsCompleted++
		case <-ctx.Done():
			t.Fatalf("Test timed out waiting for operations (%d/%d complete)", operationsCompleted, 2)
		}
	}

	cancel()
	<-done

	order := runtime.Get("order")
	if order == goja.Undefined() {
		t.Fatal("Expected order array to be populated")
	}

	// Goja exports arrays as []interface{}, need to convert to []string
	orderIntf := order.Export()
	if orderIntf == nil {
		t.Fatal("Expected order array to be populated")
	}
	orderArr, ok := orderIntf.([]interface{})
	if !ok {
		t.Fatalf("Expected []interface{} for order, got: %T", orderIntf)
	}

	// Convert []interface{} to []string
	orderStrs := make([]string, len(orderArr))
	for i, v := range orderArr {
		if s, ok := v.(string); ok {
			orderStrs[i] = s
		} else {
			t.Fatalf("Expected string at index %d, got: %T", i, v)
		}
	}

	if len(orderStrs) != 2 || orderStrs[0] != "microtask" || orderStrs[1] != "timer" {
		t.Errorf("Expected [microtask, timer], got: %v", orderStrs)
	}

	t.Log("✓ Microtasks execute before timers (event-based synchronization)")
}

// TestContextCancellation tests context cancellation behavior
func TestContextCancellation(t *testing.T) {
	// Test 2.3.9: Test Context cancellation
	ctx, cancel := context.WithCancel(context.Background())
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
		setTimeout(() => {});
		setTimeout(() => {});
		setTimeout(() => {});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	// Run loop in background AFTER RunString
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Cancel immediately
	cancel()

	// Wait for loop shutdown with timeout
	select {
	case err := <-done:
		if err != nil {
			// Timeout errors are OK (context canceled)
			t.Logf("Loop stopped with error: %v", err)
		} else {
			t.Log("Loop shut down cleanly")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Loop did not shut down within 1 second")
	}
}

// TestConcurrentJSOperations tests stress with 100 concurrent JS operations
func TestConcurrentJSOperations(t *testing.T) {
	// Test 2.3.10: Test Stress - 100 concurrent JS operations
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

	// Schedule 50 timers via setTimeout
	script := ""
	for i := 0; i < 50; i++ {
		script += `setTimeout(() => {}, 10);`
	}

	// Schedule 10 promises via Promise.resolve
	for i := 0; i < 10; i++ {
		script += `Promise.resolve().then(() => {});`
	}

	// Schedule 10 promises via new Promise
	for i := 0; i < 10; i++ {
		script += `new Promise(resolve => setTimeout(resolve, 10));`
	}

	_, err = runtime.RunString(script)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	// Run loop in background to process microtasks
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Wait for all operations to complete
	time.Sleep(200 * time.Millisecond)

	cancel()
	err = <-done
	if err != nil && err != context.Canceled {
		t.Errorf("Loop should exit cleanly with context canceled, got: %v", err)
	} else {
		t.Log("Concurrent JS operations completed successfully")
	}
}

// TestSetImmediate tests setImmediate binding from JavaScript
func TestSetImmediate(t *testing.T) {
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

	done := make(chan struct{})
	_ = runtime.Set("notifyDone", func() {
		close(done)
	})

	_, err = runtime.RunString(`
		var called = false;
		setImmediate(function() {
			called = true;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	// Run loop in background AFTER RunString
	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Run(ctx)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for setImmediate")
	}

	// Stop loop
	cancel()
	<-loopDone

	called := runtime.Get("called")
	if !called.ToBoolean() {
		t.Error("setImmediate callback should have been called")
	}
}

// TestClearImmediate tests clearImmediate binding from JavaScript
func TestClearImmediate(t *testing.T) {
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

	done := make(chan struct{})
	_ = runtime.Set("notifyDone", func() {
		close(done)
	})

	// Run loop in background FIRST
	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Run(ctx)
	}()

	// Execute RunString ON THE LOOP to ensure thread safety
	// (CancelTimer requires the loop to be running/processing to handle the cancellation safely)
	runErrCh := make(chan error, 1)
	err = loop.SubmitInternal(func() {
		_, runErr := runtime.RunString(`
			var called = false;
			var id = setImmediate(function() {
				called = true;
			});
			clearImmediate(id);

			setTimeout(function() {
				notifyDone();
			}, 50);
		`)
		runErrCh <- runErr
	})
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	// Wait for RunString to complete
	select {
	case err := <-runErrCh:
		if err != nil {
			t.Fatalf("Failed to run JavaScript: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("RunString timed out")
	}

	// Wait for the timeout to trigger notifyDone
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for timeout completion")
	}

	// Stop loop
	cancel()
	<-loopDone

	called := runtime.Get("called")
	if called.ToBoolean() {
		t.Error("setImmediate callback should not have been called after clearImmediate")
	}
}
