//go:build linux || darwin

package gojaeventloop

// Test to verify wrapped promise export behavior
import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

func TestWrappedPromiseExportBehavior(t *testing.T) {
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

	// Test 1: What does Export() return for a wrapped promise
	t.Run("ExportWrappedPromise", func(t *testing.T) {
		result, err := runtime.RunString(`
			const p = Promise.resolve(42);
			// Access the wrapped object
			const reason = p;
			reason;
		`)

		if err != nil {
			t.Fatalf("Failed to run JS: %v", err)
		}

		exported := result.Export()
		t.Logf("Exported type: %T", exported)

		// Check if it's a ChainedPromise
		if chained, ok := exported.(*goeventloop.ChainedPromise); ok {
			t.Logf("Exported is *goeventloop.ChainedPromise: %p", chained)
		} else {
			t.Logf("Exported is NOT *goeventloop.ChainedPromise")
		}
	})

	// Test 2: Does Promise.reject preserve the promise
	t.Run("RejectPromiseIdentity", func(t *testing.T) {
		result, err := runtime.RunString(`
			const p1 = Promise.resolve(42);
			const p2 = Promise.reject(p1);
			p2;
		`)

		if err != nil {
			t.Fatalf("Failed to run JS: %v", err)
		}

		exported := result.Export()
		t.Logf("P2 Exported type: %T", exported)

		// This should be a ChainedPromise (the rejected one)
		if chained, ok := exported.(*goeventloop.ChainedPromise); ok {
			t.Logf("P2 is *goeventloop.ChainedPromise")
			t.Logf("P2 state: %d", chained.State())
		} else {
			t.Logf("P2 is NOT *goeventloop.ChainedPromise")
		}
	})
}
