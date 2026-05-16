package gojaeventloop

import (
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// testEventLoopSetup creates a dormant loop for synchronous owner-only runtime
// access. Tests that exercise asynchronous work start Run after binding.
func testEventLoopSetup(t *testing.T) (*goeventloop.Loop, func()) {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	return loop, func() {
		_ = loop.Close()
	}
}

func bindRetainedEventTestSurface(t *testing.T, adapter *Adapter) {
	t.Helper()
	if err := adapter.runtime.Set("EventTarget", adapter.eventTargetConstructor); err != nil {
		t.Fatalf("install EventTarget: %v", err)
	}
	if err := adapter.runtime.Set("Event", adapter.eventConstructor); err != nil {
		t.Fatalf("install Event: %v", err)
	}
	eventTargetConstructor := adapter.runtime.Get("EventTarget").ToObject(adapter.runtime)
	eventConstructor := adapter.runtime.Get("Event").ToObject(adapter.runtime)
	if err := adapter.bindEventTargetPrototype(eventTargetConstructor); err != nil {
		t.Fatalf("bind EventTarget: %v", err)
	}
	if err := adapter.bindEventPrototype(eventConstructor); err != nil {
		t.Fatalf("bind Event: %v", err)
	}
	if err := adapter.runtime.Set("CustomEvent", adapter.customEventConstructor); err != nil {
		t.Fatalf("install CustomEvent: %v", err)
	}
	customPrototype := constructorPrototype(adapter.runtime, "CustomEvent")
	if customPrototype == nil {
		t.Fatal("CustomEvent prototype unavailable")
	}
	if err := customPrototype.SetPrototype(constructorPrototype(adapter.runtime, "Event")); err != nil {
		t.Fatalf("set CustomEvent inheritance: %v", err)
	}
	if err := adapter.bindCustomEventPrototype(adapter.runtime.Get("CustomEvent").ToObject(adapter.runtime)); err != nil {
		t.Fatalf("bind CustomEvent: %v", err)
	}
}

// ============================================================================
// EventTarget JS Binding Tests
// ============================================================================

func TestEventTarget_Constructor(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const target = new EventTarget();
		if (typeof target !== 'object') {
			throw new Error('EventTarget should be an object');
		}
		if (typeof target.addEventListener !== 'function') {
			throw new Error('addEventListener should be a function');
		}
		if (typeof target.removeEventListener !== 'function') {
			throw new Error('removeEventListener should be a function');
		}
		if (typeof target.dispatchEvent !== 'function') {
			throw new Error('dispatchEvent should be a function');
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEventTarget_AddEventListener_Basic(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();
		let called = false;

		target.addEventListener('click', function(e) {
			called = true;
		});

		const event = new Event('click');
		target.dispatchEvent(event);

		called;
	`)
	if err != nil {
		t.Fatal(err)
	}

	if !result.ToBoolean() {
		t.Error("Listener should have been called")
	}
}

func TestEventTarget_AddEventListener_MultipleListeners(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();
		const order = [];

		target.addEventListener('test', function(e) {
			order.push(1);
		});
		target.addEventListener('test', function(e) {
			order.push(2);
		});
		target.addEventListener('test', function(e) {
			order.push(3);
		});

		target.dispatchEvent(new Event('test'));

		order.join(',');
	`)
	if err != nil {
		t.Fatal(err)
	}

	if result.String() != "1,2,3" {
		t.Errorf("Expected '1,2,3', got '%s'", result.String())
	}
}

func TestEventTarget_AddEventListener_Once(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();
		let callCount = 0;

		target.addEventListener('click', function(e) {
			callCount++;
		}, { once: true });

		target.dispatchEvent(new Event('click'));
		target.dispatchEvent(new Event('click'));
		target.dispatchEvent(new Event('click'));

		callCount;
	`)
	if err != nil {
		t.Fatal(err)
	}

	if result.ToInteger() != 1 {
		t.Errorf("Once listener should be called exactly once, got %d", result.ToInteger())
	}
}

func TestEventTarget_RemoveEventListener(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();
		let called = false;

		const handler = function(e) {
			called = true;
		};

		target.addEventListener('click', handler);
		target.removeEventListener('click', handler);

		target.dispatchEvent(new Event('click'));

		called;
	`)
	if err != nil {
		t.Fatal(err)
	}

	if result.ToBoolean() {
		t.Error("Listener should not be called after removal")
	}
}

func TestEventTarget_RemoveEventListener_DifferentFunction(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();
		let called = false;

		target.addEventListener('click', function(e) {
			called = true;
		});

		// Try to remove with different function - should not work
		target.removeEventListener('click', function(e) {});

		target.dispatchEvent(new Event('click'));

		called;
	`)
	if err != nil {
		t.Fatal(err)
	}

	if !result.ToBoolean() {
		t.Error("Listener should still be called (different function reference)")
	}
}
