// Tests for critical bug fixes in the Promise implementation.

package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// TestCriticalFixes_Verification verifies Promise identity and reject semantics.
func TestCriticalFixes_Verification(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	// Promise.resolve preserves native Promise identity.
	val1, err := runtime.RunString(`
		(() => {
			const p1 = Promise.resolve(1);
			return Promise.resolve(p1) === p1;
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

			return caughtValue === p1;
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
