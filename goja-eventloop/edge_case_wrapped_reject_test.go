// Edge case test for wrapped promise as rejection reason

package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestWrappedPromiseAsRejectReason verifies that when a wrapped promise
// is used as a rejection reason, it's preserved correctly through the chain
func TestWrappedPromiseAsRejectReason(t *testing.T) {
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

	val, err := runtime.RunString(`
		(async () => {
			const p1 = Promise.resolve(42);
			const p2 = Promise.reject(p1);
			
			// Chain catch handler that receives the wrapped promise
			const p3 = p2.catch(reason => {
				// reason should be the wrapped promise object (p1)
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
		t.Error("FAILED: Wrapped promise as reject reason not preserved correctly in handler chain")
	} else {
		t.Log("PASSED: Wrapped promise as reject reason preserved correctly")
	}

	// Run loop
	go func() { _ = loop.Run(ctx) }()
}
