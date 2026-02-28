// Tests for critical bug fixes in the Promise implementation.

package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestCriticalFixes_Verification verifies Promise identity and reject semantics.
func TestCriticalFixes_Verification(t *testing.T) {
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
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	// Promise identity preservation (no double-wrapping)
	val1, err := runtime.RunString(`
		(async () => {
			const p1 = Promise.resolve(1);
			const p2 = Promise.all([p1]);
			const results = await p2;
			return results[0] === p1;
		})()
	`)
	if err != nil {
		t.Fatalf("Promise identity test failed to execute: %v", err)
	}
	if !val1.ToBoolean() {
		t.Error("Promise identity not preserved (double-wrapping issue)")
	}

	// Promise.reject(promise) should reject with the promise itself
	val2, err := runtime.RunString(`
		(async () => {
			const p1 = new Promise(r => r(1));
			const caughtReject = Promise.reject(p1);

			await p1;

			let caughtValue = null;
			try {
				await caughtReject;
			} catch (reason) {
				caughtValue = reason;
			}

			return caughtValue !== null &&
				typeof caughtValue === 'object' &&
				'_internalPromise' in caughtValue;
		})()
	`)
	if err != nil {
		t.Fatalf("Promise.reject test failed to execute: %v", err)
	}
	if !val2.ToBoolean() {
		t.Error("Promise.reject semantics incorrect")
	}

	go func() { _ = loop.Run(ctx) }()
	time.Sleep(100 * time.Millisecond)
}
