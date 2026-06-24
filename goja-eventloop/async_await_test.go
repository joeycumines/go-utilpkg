package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// TestAsyncAwaitDrainsViaEventLoop verifies that goja's native async/await
// promise jobs are routed to the event loop's microtask queue by the
// PromiseJobEnqueuer hook. Without the hook, the async continuation after
// `await` would be stuck in goja's internal jobQueue (drained only by
// leave()), and loop.Run() would never execute it.
func TestAsyncAwaitDrainsViaEventLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind adapter: %v", err)
	}

	resultCh := make(chan goja.Value, 1)
	_ = runtime.Set("reportResult", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0)
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		async function compute() {
			const value = await Promise.resolve(42);
			reportResult(value);
			return value;
		}
		compute();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case result := <-resultCh:
		if result.ToInteger() != 42 {
			t.Errorf("expected 42, got %v", result)
		}
	case <-ctx.Done():
		t.Fatal("timeout: async/await continuation did not drain via event loop")
	}
}

// TestAsyncAwaitChain verifies that multiple sequential awaits in a single
// async function all resolve through the event loop microtask queue.
func TestAsyncAwaitChain(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind adapter: %v", err)
	}

	resultCh := make(chan int64, 1)
	_ = runtime.Set("reportResult", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0).ToInteger()
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		async function chain() {
			const a = await Promise.resolve(1);
			const b = await Promise.resolve(a + 1);
			const c = await Promise.resolve(b + 1);
			reportResult(c);
			return c;
		}
		chain();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case result := <-resultCh:
		if result != 3 {
			t.Errorf("expected 3, got %d", result)
		}
	case <-ctx.Done():
		t.Fatal("timeout: async chain did not complete")
	}
}

// TestAsyncAwaitRejection verifies that rejected promises propagate correctly
// through async/await when jobs are routed via the event loop.
func TestAsyncAwaitRejection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind adapter: %v", err)
	}

	resultCh := make(chan string, 1)
	_ = runtime.Set("reportResult", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0).String()
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		async function fail() {
			try {
				await Promise.reject("test error");
				reportResult("NO_THROW");
			} catch (e) {
				reportResult(String(e));
			}
		}
		fail();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case result := <-resultCh:
		if result != "test error" {
			t.Errorf("expected 'test error', got %q", result)
		}
	case <-ctx.Done():
		t.Fatal("timeout: async rejection did not propagate")
	}
}

// TestAsyncAwaitMixedWithAdapterPromise verifies that async/await (native
// goja promises) interoperate correctly with the adapter's ChainedPromise-based
// Promise override. An async function awaiting a Promise created via the adapter
// global should resolve correctly.
func TestAsyncAwaitMixedWithAdapterPromise(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind adapter: %v", err)
	}

	resultCh := make(chan int64, 1)
	_ = runtime.Set("reportResult", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0).ToInteger()
		return goja.Undefined()
	})

	// The async function uses goja's native async/await (routed via the
	// PromiseJobEnqueuer hook), while Promise.resolve uses the adapter's
	// ChainedPromise implementation. The thenable interop path (resolveThenable)
	// bridges the two promise systems.
	_, err = runtime.RunString(`
		async function mixed() {
			const result = await Promise.resolve(99);
			reportResult(result);
			return result;
		}
		mixed();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case result := <-resultCh:
		if result != 99 {
			t.Errorf("expected 99, got %d", result)
		}
	case <-ctx.Done():
		t.Fatal("timeout: mixed native/adapter promise did not resolve")
	}
}
