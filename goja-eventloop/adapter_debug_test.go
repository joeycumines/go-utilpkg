package gojaeventloop_test

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	eventloop "github.com/joeycumines/go-eventloop"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// TestPromiseChainSimple simplifies the test to isolate the issue
func TestPromiseChainSimple(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	// Run the loop
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Simple chain test - check if methods exist
	_, err = runtime.RunString(`
		// Create first promise
		let p1 = new Promise((resolve) => {
			resolve(1);
		});
		
		// Check if .then method exists on p1
		if (typeof p1.then !== 'function') {
			throw new Error('p1.then is not a function: ' + typeof p1.then);
		}
		
		// Chain first .then()
		let p2 = p1.then(x => {
			return x + 1;
		});
		
		// Check if .then method exists on p2
		if (typeof p2.then !== 'function') {
			throw new Error('p2.then is not a function: ' + typeof p2.then);
		}
		
		// Chain second .then()
		let p3 = p2.then(x => {
			return x * 2;
		});
		
		// Check if .then method exists on p3
		if (typeof p3.then !== 'function') {
			throw new Error('p3.then is not a function: ' + typeof p3.then);
		}
	`)
	if err != nil {
		t.Logf("JavaScript error checking .then methods: %v", err)
	}
	if err != nil {
		t.Logf("JavaScript run error: %v", err)
	}

	// Wait for microtasks
	time.Sleep(100 * time.Millisecond)

	t.Logf("Final result: %v (type: %T)", result, result)
	if result == nil {
		t.Log("Result is nil (expected if error)")
	}

	cancel()
	<-done
}
