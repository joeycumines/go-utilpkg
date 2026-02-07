package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestReproThenable verifies if Promise.resolve handles "thenables" correctly
// (Promise/A+ 2.3.3)
func TestReproThenable(t *testing.T) {
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

	resultCh := make(chan interface{}, 1)
	_ = runtime.Set("notifyResult", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0).Export()
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		const thenable = {
			then: (resolve, reject) => {
				setTimeout(() => resolve("resolved from thenable"), 10);
			}
		};

		Promise.resolve(thenable).then(val => {
			notifyResult(val);
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	go func() { _ = loop.Run(ctx) }()

	select {
	case export := <-resultCh:
		if export != "resolved from thenable" {
			t.Errorf("Expected 'resolved from thenable', got: %v (type %T)", export, export)
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for thenable resolution")
	}
}

// TestReproIterable verifies if Promise.all handles generic iterables (e.g. Set)
func TestReproIterable(t *testing.T) {
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

	resultCh := make(chan interface{}, 1)
	_ = runtime.Set("notifyResult", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0).Export()
		return goja.Undefined()
	})
	errCh := make(chan string, 1)
	_ = runtime.Set("notifyError", func(call goja.FunctionCall) goja.Value {
		errCh <- call.Argument(0).String()
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		const s = new Set();
		s.add(Promise.resolve(1));
		s.add(2);

		// Note: Promise.all takes an iterable. Set is iterable.
		Promise.all(s).then(val => {
			notifyResult(val);
		}).catch(err => {
			notifyError("ERROR: " + err.message);
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	go func() { _ = loop.Run(ctx) }()

	select {
	case export := <-resultCh:
		// Expect array [1, 2]
		if arr, ok := export.([]interface{}); ok {
			if len(arr) != 2 {
				t.Errorf("Expected length 2, got %d", len(arr))
			}
		} else {
			t.Errorf("Expected array, got: %v (type %T)", export, export)
		}
	case errMsg := <-errCh:
		t.Errorf("Got error: %s", errMsg)
	case <-ctx.Done():
		t.Fatal("Timeout waiting for Promise.all(Set)")
	}
}
