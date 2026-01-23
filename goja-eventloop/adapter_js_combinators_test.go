// Copyright 2025 Joseph Cumines
//
// JavaScript-level tests for Promise combinators (All, Race, AllSettled, Any)
// These tests verify the combinators work when called from JavaScript code

package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ============================================================================
// JavaScript-Level Tests for Promise.all
// ============================================================================

// CRITICAL #4: Test Promise.all from JavaScript
func TestPromiseAllFromJavaScript(t *testing.T) {
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

	// Test Promise.all with array of resolved promises
	done := make(chan struct{})
	_ = runtime.Set("notifyDone", func() {
		close(done)
	})

	_, err = runtime.RunString(`
		const ps = [
			Promise.resolve(1),
			Promise.resolve(2),
			Promise.resolve(3)
		];
		let result;

			Promise.all(ps).then(values => {
			result = values;
			notifyDone();
		}).catch(err => {
			console.error("Promise.all failed:", err);
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for Promise.all to complete")
	}

	result := runtime.Get("result")
	if result == nil || result == goja.Undefined() {
		t.Fatal("Promise.all should resolve with array of values, got undefined")
	}

	values := result.Export()
	resultArr, ok := values.([]interface{})
	if !ok {
		t.Fatalf("Expected []interface{}, got: %T", values)
	}

	if len(resultArr) != 3 {
		t.Errorf("Expected 3 values, got: %d", len(resultArr))
	}

	expected := []interface{}{int64(1), int64(2), int64(3)}
	for i, v := range resultArr {
		if v != expected[i] {
			t.Errorf("Index %d: expected %v, got %v", i, expected[i], v)
		}
	}

	t.Log("✓ Promise.all works from JavaScript")
}

// CRITICAL #4: Test Promise.all with rejection from JavaScript
func TestPromiseAllWithRejectionFromJavaScript(t *testing.T) {
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
		const ps = [
			Promise.resolve(1),
			Promise.reject(new Error("test error")),
			Promise.resolve(3)
		];
		let errorResult;

		Promise.all(ps).catch(err => {
			errorResult = err.message;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for Promise.all rejection")
	}

	errorResult := runtime.Get("errorResult")
	if errorResult == nil || errorResult == goja.Undefined() {
		t.Fatal("Promise.all should reject with first error, got undefined")
	}

	errMsg := errorResult.Export()
	if errMsg != "test error" {
		t.Errorf("Expected 'test error', got: %v", errMsg)
	}

	t.Log("✓ Promise.all rejection works from JavaScript")
}

// ============================================================================
// JavaScript-Level Tests for Promise.race
// ============================================================================

// CRITICAL #4: Test Promise.race from JavaScript
func TestPromiseRaceFromJavaScript(t *testing.T) {
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
		const ps = [
			new Promise(resolve => setTimeout(() => resolve(1), 100)),
			Promise.resolve(2),  // This should win
			new Promise(resolve => setTimeout(() => resolve(3), 100))
		];
		let result;

		Promise.race(ps).then(value => {
			result = value;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for Promise.race to complete")
	}

	result := runtime.Get("result")
	if result == nil || result == goja.Undefined() {
		t.Fatal("Promise.race should resolve with winner, got undefined")
	}

	val := result.Export()
	if val != int64(2) {
		t.Errorf("Expected 2 (fastest promise), got: %v", val)
	}

	t.Log("✓ Promise.race works from JavaScript")
}

// ============================================================================
// JavaScript-Level Tests for Promise.allSettled
// ============================================================================

// CRITICAL #4: Test Promise.allSettled from JavaScript
func TestPromiseAllSettledFromJavaScript(t *testing.T) {
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
		const ps = [
			Promise.resolve(1),
			Promise.reject(new Error("err")),
			Promise.resolve(3)
		];
		let result;

		Promise.allSettled(ps).then(values => {
			result = values;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for Promise.allSettled to complete")
	}

	result := runtime.Get("result")
	if result == nil || result == goja.Undefined() {
		t.Fatal("Promise.allSettled should resolve with status objects, got undefined")
	}

	values := result.Export()
	resultArr, ok := values.([]interface{})
	if !ok {
		t.Fatalf("Expected []interface{}, got: %T", values)
	}

	if len(resultArr) != 3 {
		t.Errorf("Expected 3 status objects, got: %d", len(resultArr))
	}

	// Check first result: {status: "fulfilled", value: 1}
	first, ok := resultArr[0].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map for first result, got: %T", resultArr[0])
	}
	if first["status"] != "fulfilled" || first["value"] != int64(1) {
		t.Errorf("First result incorrect: %v", first)
	}

	// Check second result: {status: "rejected", reason: Error}
	second, ok := resultArr[1].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map for second result, got: %T", resultArr[1])
	}
	if second["status"] != "rejected" {
		t.Errorf("Second status should be 'rejected', got: %v", second["status"])
	}

	t.Log("✓ Promise.allSettled works from JavaScript")
}

// ============================================================================
// JavaScript-Level Tests for Promise.any
// ============================================================================

// CRITICAL #4: Test Promise.any from JavaScript
func TestPromiseAnyFromJavaScript(t *testing.T) {
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
		const ps = [
			Promise.reject(new Error("err1")),
			Promise.resolve(2),
			Promise.reject(new Error("err2"))
		];
		let result;

		Promise.any(ps).then(value => {
			result = value;
			notifyDone();
		}).catch(err => {
			result = "CAUGHT: " + err.message;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for Promise.any to complete")
	}

	result := runtime.Get("result")
	if result == nil || result == goja.Undefined() {
		t.Fatal("Promise.any should resolve with first fulfillment, got undefined")
	}

	val := result.Export()
	if val != int64(2) {
		t.Errorf("Expected 2 (first fulfillment), got: %v", val)
	}

	t.Log("✓ Promise.any works from JavaScript")
}

// CRITICAL #4: Test Promise.any with all rejected from JavaScript
func TestPromiseAnyAllRejectedFromJavaScript(t *testing.T) {
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
		const ps = [
			Promise.reject(new Error("err1")),
			Promise.reject(new Error("err2"))
		];
		let errorResult;

		Promise.any(ps).catch(err => {
			errorResult = err.message;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for Promise.any rejection")
	}

	errorResult := runtime.Get("errorResult")
	if errorResult == nil || errorResult == goja.Undefined() {
		t.Fatal("Promise.any should reject with AggregateError, got undefined")
	}

	errMsg := errorResult.Export()
	if errMsg != "All promises were rejected" {
		t.Errorf("Expected AggregateError message, got: %v", errMsg)
	}

	t.Log("✓ Promise.any all-rejected case works from JavaScript")
}

// CRITICAL #2: Test .then() chain with proper result values
func TestPromiseThenChainFromJavaScript(t *testing.T) {
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

	// Test multi-step promise chain that should compute (1 + 1) * 2 = 4
	_, err = runtime.RunString(`
		let p = Promise.resolve(1);
		let result;

		p.then(x => {
			console.log("First then, x=", x);
			return x + 1;
		}).then(x => {
			console.log("Second then, x=", x);
			return x * 2;
		}).then(x => {
			console.log("Third then, x=", x);
			result = x;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for promise chain")
	}

	result := runtime.Get("result")
	if result == nil || result == goja.Undefined() {
		t.Fatal("Promise chain should resolve with result, got undefined")
	}

	val := result.Export()
	if val != int64(4) {
		t.Errorf("Expected promise chain to compute 4, got: %v", val)
	}

	t.Log("✓ Promise .then() chain works correctly from JavaScript")
}

// CRITICAL #3: Test that errors in .then() handlers don't panic
func TestPromiseThenErrorHandlingFromJavaScript(t *testing.T) {
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
		Promise.resolve(1)
			.then(() => {
				throw new Error("test error in handler");
			})
			.then(() => {
				// This should not run
				result = "should not see this";
			})
			.catch(err => {
				result = err.message;
				notifyDone();
			});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for error handling")
	}

	result := runtime.Get("result")
	if result == nil || result == goja.Undefined() {
		t.Fatal("Error should be caught by .catch(), got undefined")
	}

	errMsg := result.Export()
	if errMsg != "test error in handler" {
		t.Errorf("Expected 'test error in handler', got: %v", errMsg)
	}

	t.Log("✓ Promise error handling works correctly from JavaScript")
}
