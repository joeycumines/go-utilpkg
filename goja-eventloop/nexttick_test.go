package gojaeventloop

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// EXPAND-020: process.nextTick() JS Binding Tests
// ===============================================

func TestProcessNextTick_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	done := make(chan struct{})
	var result string
	var mu sync.Mutex

	runtime.Set("setResult", func(call goja.FunctionCall) goja.Value {
		mu.Lock()
		result = call.Argument(0).String()
		mu.Unlock()
		close(done)
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		process.nextTick(function() {
			setResult("called");
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	go func() {
		_ = loop.Run(context.Background())
	}()

	select {
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		if result != "called" {
			t.Errorf("Expected 'called', got: %s", result)
		}
	case <-time.After(time.Second):
		t.Error("Callback did not execute in time")
	}
}

func TestProcessNextTick_RunsBeforePromise(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	done := make(chan struct{})
	var order []string
	var mu sync.Mutex

	runtime.Set("recordOrder", func(call goja.FunctionCall) goja.Value {
		mu.Lock()
		order = append(order, call.Argument(0).String())
		if len(order) == 2 {
			close(done)
		}
		mu.Unlock()
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		// Promise.then is a microtask
		Promise.resolve().then(function() {
			recordOrder("promise");
		});

		// nextTick should run before promise
		process.nextTick(function() {
			recordOrder("nextTick");
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	go func() {
		_ = loop.Run(context.Background())
	}()

	select {
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		if len(order) != 2 {
			t.Fatalf("Expected 2 items, got %d", len(order))
		}
		if order[0] != "nextTick" {
			t.Errorf("Expected 'nextTick' first, got order: %v", order)
		}
		if order[1] != "promise" {
			t.Errorf("Expected 'promise' second, got order: %v", order)
		}
	case <-time.After(time.Second):
		t.Error("Callbacks did not execute in time")
	}
}

func TestProcessNextTick_Multiple(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	done := make(chan struct{})
	var order []int
	var mu sync.Mutex

	runtime.Set("recordOrder", func(call goja.FunctionCall) goja.Value {
		mu.Lock()
		order = append(order, int(call.Argument(0).ToInteger()))
		if len(order) == 3 {
			close(done)
		}
		mu.Unlock()
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		process.nextTick(function() { recordOrder(1); });
		process.nextTick(function() { recordOrder(2); });
		process.nextTick(function() { recordOrder(3); });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	go func() {
		_ = loop.Run(context.Background())
	}()

	select {
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		expected := []int{1, 2, 3}
		for i, v := range expected {
			if order[i] != v {
				t.Errorf("At index %d: expected %d, got %d", i, v, order[i])
			}
		}
	case <-time.After(time.Second):
		t.Error("Callbacks did not execute in time")
	}
}

func TestProcessNextTick_NoArgumentError(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	_, err = runtime.RunString(`
		try {
			process.nextTick();
		} catch (e) {
			// Expected TypeError
		}
	`)
	// Should not cause a Go panic
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===============================================
// EXPAND-021: delay() JS Binding Tests
// ===============================================

func TestDelay_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	done := make(chan struct{})
	var resolved bool
	var mu sync.Mutex

	runtime.Set("onResolved", func(call goja.FunctionCall) goja.Value {
		mu.Lock()
		resolved = true
		mu.Unlock()
		close(done)
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		delay(50).then(function() {
			onResolved();
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	go func() {
		_ = loop.Run(context.Background())
	}()

	start := time.Now()
	select {
	case <-done:
		elapsed := time.Since(start)
		mu.Lock()
		defer mu.Unlock()
		if !resolved {
			t.Error("delay() did not resolve")
		}
		if elapsed < 40*time.Millisecond {
			t.Errorf("delay() resolved too quickly: %v", elapsed)
		}
	case <-time.After(time.Second):
		t.Error("delay() did not resolve in time")
	}
}

func TestDelay_ZeroMs(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	done := make(chan struct{})

	runtime.Set("onDone", func(call goja.FunctionCall) goja.Value {
		close(done)
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		delay(0).then(onDone);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	go func() {
		_ = loop.Run(context.Background())
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Error("delay(0) did not resolve in time")
	}
}

func TestDelay_Chaining(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	done := make(chan struct{})
	var order []string
	var mu sync.Mutex

	runtime.Set("recordOrder", func(call goja.FunctionCall) goja.Value {
		mu.Lock()
		order = append(order, call.Argument(0).String())
		if len(order) == 2 {
			close(done)
		}
		mu.Unlock()
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		delay(10)
			.then(function() {
				recordOrder("first");
				return "value";
			})
			.then(function(v) {
				recordOrder("second");
			});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	go func() {
		_ = loop.Run(context.Background())
	}()

	select {
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		if len(order) != 2 {
			t.Fatalf("Expected 2 calls, got %d", len(order))
		}
		if order[0] != "first" || order[1] != "second" {
			t.Errorf("Unexpected order: %v", order)
		}
	case <-time.After(time.Second):
		t.Error("Chain did not complete in time")
	}
}

func TestDelay_ReturnsPromise(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	result, err := runtime.RunString(`
		var p = delay(0);
		typeof p.then === 'function' && typeof p.catch === 'function';
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !result.ToBoolean() {
		t.Error("delay() should return a promise-like object with then and catch methods")
	}
}

func TestDelay_NegativeValueTreatedAsZero(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	done := make(chan struct{})

	runtime.Set("onDone", func(call goja.FunctionCall) goja.Value {
		close(done)
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		delay(-100).then(onDone);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	go func() {
		_ = loop.Run(context.Background())
	}()

	select {
	case <-done:
		// Success - negative delay should be treated as 0
	case <-time.After(time.Second):
		t.Error("delay(-100) did not resolve in time")
	}
}
