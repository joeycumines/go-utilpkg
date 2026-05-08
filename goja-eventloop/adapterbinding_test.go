package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// Adapter construction and JavaScript global binding coverage.

// TestAdapter_New_NilLoop verifies the static nil-loop contract.
func TestAdapter_New_NilLoop(t *testing.T) {
	runtime := goja.New()
	defer assertAdapterPanic(t, "nil loop")
	_, _ = New(nil, runtime)
}

// TestAdapter_New_NilRuntime verifies the static nil-runtime contract.
func TestAdapter_New_NilRuntime(t *testing.T) {
	loop := goeventloop.New()
	defer loop.Shutdown(context.Background())

	defer assertAdapterPanic(t, "nil runtime")
	_, _ = New(loop, nil)
}

func assertAdapterPanic(t *testing.T, label string) {
	t.Helper()
	if recover() == nil {
		t.Fatalf("%s did not panic", label)
	}
}

// TestAdapter_Bind_AllGlobals verifies all globals are bound.
func TestAdapter_Bind_AllGlobals(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop := goeventloop.New()
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Verify all expected globals are set
	globals := []string{
		"setTimeout",
		"clearTimeout",
		"setInterval",
		"clearInterval",
		"queueMicrotask",
		"setImmediate",
		"clearImmediate",
		"Promise",
	}

	for _, name := range globals {
		val := rt.Get(name)
		if val == nil || goja.IsUndefined(val) {
			t.Errorf("Global %q should be defined", name)
		}
	}
	if value := rt.Get("consumeIterable"); value != nil && !goja.IsUndefined(value) {
		t.Error("internal consumeIterable helper leaked into the global scope")
	}
}

// TestAdapter_Bind_PromiseStatics verifies Promise static methods.
func TestAdapter_Bind_PromiseStatics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop := goeventloop.New()
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Verify Promise static methods
	statics := []string{
		"resolve",
		"reject",
		"all",
		"race",
		"allSettled",
		"any",
		"prototype",
	}

	for _, name := range statics {
		val, err := rt.RunString("Promise." + name)
		if err != nil {
			t.Errorf("Promise.%s should exist: %v", name, err)
		}
		if val == nil || goja.IsUndefined(val) {
			t.Errorf("Promise.%s should not be undefined", name)
		}
	}
}

// TestAdapter_setTimeout_NilFunction verifies behavior with nil function.
func TestAdapter_setTimeout_NilFunction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop := goeventloop.New()
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// setTimeout with null should throw TypeError
	_, err = rt.RunString("setTimeout(null, 100)")
	if err == nil {
		t.Error("setTimeout(null) should throw TypeError")
	}
}

// TestAdapter_setInterval_NilFunction verifies behavior with nil function.
func TestAdapter_setInterval_NilFunction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop := goeventloop.New()
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// setInterval with null should throw TypeError
	_, err = rt.RunString("setInterval(null, 100)")
	if err == nil {
		t.Error("setInterval(null) should throw TypeError")
	}
}

// TestAdapter_queueMicrotask_NilFunction verifies behavior with nil function.
func TestAdapter_queueMicrotask_NilFunction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop := goeventloop.New()
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// queueMicrotask with null should throw TypeError
	_, err = rt.RunString("queueMicrotask(null)")
	if err == nil {
		t.Error("queueMicrotask(null) should throw TypeError")
	}
}

// TestAdapter_setImmediate_NilFunction verifies behavior with nil function.
func TestAdapter_setImmediate_NilFunction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop := goeventloop.New()
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// setImmediate with null should throw TypeError
	_, err = rt.RunString("setImmediate(null)")
	if err == nil {
		t.Error("setImmediate(null) should throw TypeError")
	}
}

// TestAdapterSetTimeoutNegativeDelayUsesNodeMinimum verifies the retained Node
// timer coercion without duplicating the complete timer-delay matrix.
func TestAdapterSetTimeoutNegativeDelayUsesNodeMinimum(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop := goeventloop.New()
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	val, err := rt.RunString(`
		(() => {
			const handle = setTimeout(function() {}, -1);
			const delay = handle._idleTimeout;
			clearTimeout(handle);
			return delay;
		})()
	`)
	if err != nil {
		t.Fatalf("setTimeout negative delay: %v", err)
	}
	if got := val.ToInteger(); got != 1 {
		t.Fatalf("setTimeout negative _idleTimeout = %d, want 1", got)
	}
}
