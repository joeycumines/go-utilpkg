// Edge case test for a native Promise used as a rejection reason.

package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// TestNativePromiseAsRejectReason verifies that a native Promise rejection
// reason preserves identity through the chain.
func TestNativePromiseAsRejectReason(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	val, err := runtime.RunString(`
		(async () => {
			const p1 = Promise.resolve(42);
			const p2 = Promise.reject(p1);

			// Chain a catch handler that receives the native Promise.
			const p3 = p2.catch(reason => {
				// The reason is the exact Promise object (p1).
				return reason.then(v => v + 100);
			});

			const result = await p3;
			return result === 142;  // 42 + 100 = 142
		})()
	`)
	if err != nil {
		t.Fatalf("Failed to execute: %v", err)
	}

	result := val.ToBoolean()
	if !result {
		t.Error("native Promise rejection reason was not preserved in the handler chain")
	} else {
		t.Log("native Promise rejection reason preserved correctly")
	}

	// Run loop
	go func() { _ = loop.Run(ctx) }()
}
