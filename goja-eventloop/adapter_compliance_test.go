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

	done := make(chan struct{})
	_ = runtime.Set("notifyDone", func() {
		close(done)
	})

	_, err = runtime.RunString(`
		let result;
		const thenable = {
			then: (resolve, reject) => {
				setTimeout(() => resolve("resolved from thenable"), 10);
			}
		};

		Promise.resolve(thenable).then(val => {
			result = val;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for thenable resolution")
	}

	result := runtime.Get("result")
	export := result.Export()
	if export != "resolved from thenable" {
		t.Errorf("Expected 'resolved from thenable', got: %v (type %T)", export, export)
		// Check if it returned the object itself
		if obj, ok := export.(map[string]interface{}); ok {
			if _, hasThen := obj["then"]; hasThen {
				t.Log("Failure confirmed: Promise.resolve returned the thenable object instead of waiting for it")
			}
		}
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

	done := make(chan struct{})
	_ = runtime.Set("notifyDone", func() {
		close(done)
	})

	_, err = runtime.RunString(`
		let result;
		const s = new Set();
		s.add(Promise.resolve(1));
		s.add(2);

		// Note: Promise.all takes an iterable. Set is iterable.
		Promise.all(s).then(val => {
			result = val;
			notifyDone();
		}).catch(err => {
			result = "ERROR: " + err.message;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for Promise.all(Set)")
	}

	result := runtime.Get("result")
	export := result.Export()

	// Expect array [1, 2]
	if arr, ok := export.([]interface{}); ok {
		if len(arr) != 2 {
			t.Errorf("Expected length 2, got %d", len(arr))
		}
	} else {
		t.Errorf("Expected array, got: %v (type %T)", export, export)
		t.Log("Failure confirmed: Likely TypeError or empty array if iterable not supported")
	}
}
