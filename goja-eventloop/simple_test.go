//go:build linux || darwin

package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestPromiseRejectSimple - simple test without console.log statements that might fail if console not bound
func TestPromiseRejectSimple(t *testing.T) {
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

	// Simple test: Verify Promise.reject(promise) doesn't crash
	testDone := make(chan bool, 1)
	resultStore := make(chan string, 1)

	_ = runtime.Set("testDone", func(result string) {
		resultStore <- result
		testDone <- true
	})

	_, err = runtime.RunString(`
		const p1 = Promise.resolve(42);
		const p2 = Promise.reject(p1);

		p2.catch(reason => {
			let result = "FAIL";
			if (typeof reason.then === "function") {
				result = "THEN_FOUND";
			}
			testDone(result);
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case result := <-resultStore:
		t.Logf("Test result: %s", result)
		if result == "THEN_FOUND" {
			t.Log("PASS: Promise.reject(promise) preserved promise with .then method")
		} else {
			t.Errorf("FAIL: Expected THEN_FOUND, got %s", result)
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for promise rejection to complete")
	}
}
