package gojaeventloop_test

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	eventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja-eventloop"
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

	// Simple chain test
	result, err := runtime.RunString(`
		let result;
		new Promise((resolve) => {
			resolve(1);
		}).then(x => {
			console.log("Step 1: x=", x);
			return x + 1;
		}).then(x => {
			console.log("Step 2: x=", x);
			return x * 2;
		}).then(x => {
			console.log("Step 3: x=", x);
			result = x;
		});
		result;
	`)
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
